import { describe, expect, test } from "vitest";

import registry from "@/generated/commands.json";
import { type BuildPhase, isPhaseAtLeast } from "../boot/phase";
import {
	addClipOperations,
	captionOperations,
	splitOperations,
	trimOperations,
} from "./builders";
import { ACTIONS, ANY_COMMAND, COVERAGE, type EntryPoint } from "./coverage";

interface RegistryEntry {
	name: string;
	internal: boolean;
	phase: string;
}

const commands = registry as RegistryEntry[];
const byName = new Map(commands.map((c) => [c.name, c]));

// The build phase this editor ships: ALPHA (SDD-012 section 9.3 rule 3).
const BUILD: BuildPhase = "alpha";

const LETTER: Readonly<Record<string, BuildPhase>> = {
	P: "prototype",
	A: "alpha",
	B: "beta",
};

type Coverage = Readonly<Record<string, readonly EntryPoint[]>>;

// The four rules of SDD-012 section 9.3, as a function so that the negative
// fixtures below can run them on a damaged declaration.
function coverageProblems(
	list: readonly RegistryEntry[],
	coverage: Coverage,
	phase: BuildPhase,
): string[] {
	const problems: string[] = [];
	const names = new Set(list.map((c) => c.name));
	for (const c of list) {
		if (c.internal) continue;
		const entries = coverage[c.name] ?? [];
		if (entries.length === 0) {
			problems.push(`${c.name} has no entry point`);
			continue;
		}
		if (isPhaseAtLeast(c.phase as BuildPhase, phase)) {
			// The catalogue phase is at or before the build phase: rule 3.
			const usable = entries.filter((e) =>
				isPhaseAtLeast(LETTER[e.phase], phase),
			);
			if (usable.length === 0)
				problems.push(
					`${c.name} has no entry point of phase ${phase} or earlier`,
				);
			if (
				isPhaseAtLeast("alpha", phase) &&
				!entries.some((e) => e.kind === "palette")
			) {
				problems.push(`${c.name} has no palette entry`);
			}
		}
	}
	for (const name of Object.keys(coverage)) {
		if (!names.has(name)) problems.push(`${name} is not in the registry`);
		for (const entry of coverage[name]) {
			const sends = ACTIONS[entry.action];
			if (sends === undefined)
				problems.push(`${entry.action} is not an action`);
			else if (!sends.includes(ANY_COMMAND) && !sends.includes(name)) {
				problems.push(`${entry.action} does not send ${name}`);
			}
		}
	}
	return problems;
}

describe("command coverage (SDD-012 section 9.3)", () => {
	test("the declaration of the committed registry is complete and consistent", () => {
		expect(coverageProblems(commands, COVERAGE, BUILD)).toEqual([]);
	});

	test("every non-internal command has a key: 47 of 48", () => {
		const keys = commands.filter((c) => !c.internal).map((c) => c.name);
		expect(Object.keys(COVERAGE).sort()).toEqual(keys.sort());
		expect(keys).toHaveLength(47);
	});

	test("every key of COVERAGE is a non-internal command of the registry", () => {
		for (const name of Object.keys(COVERAGE)) {
			expect(byName.has(name), `${name} is not in the registry`).toBe(true);
			expect(byName.get(name)?.internal, name).toBe(false);
		}
	});

	test("every command the Prototype UI sends keeps its entry point of phase P", () => {
		for (const name of [
			"clip.add",
			"track.add",
			"item.split",
			"item.trim",
			"caption.add",
		]) {
			expect(
				(COVERAGE[name] ?? []).some((e) => e.phase === "P"),
				name,
			).toBe(true);
		}
	});

	test("negative: deleting one key of COVERAGE fails with the command name", () => {
		const { "item.move": _removed, ...rest } = COVERAGE;
		expect(coverageProblems(commands, rest, BUILD)).toContain(
			"item.move has no entry point",
		);
	});

	test("negative: a command added to the catalogue with no key fails", () => {
		const grown = [
			...commands,
			{ name: "clip.set_example", internal: false, phase: "alpha" },
		];
		expect(coverageProblems(grown, COVERAGE, BUILD)).toContain(
			"clip.set_example has no entry point",
		);
	});

	test("negative: a command with no palette entry fails in the ALPHA build", () => {
		const bare = { ...COVERAGE, "text.add": [] as EntryPoint[] };
		bare["text.add"] = [
			{ kind: "gesture", action: "timeline.split", phase: "P" },
		];
		const problems = coverageProblems(commands, bare, BUILD);
		expect(problems).toContain("text.add has no palette entry");
	});

	test("negative: a stale key and an unknown action fail", () => {
		const stale = {
			...COVERAGE,
			"gone.command": [
				{ kind: "palette", action: "palette.run", phase: "A" } as EntryPoint,
			],
			"item.move": [
				{ kind: "palette", action: "no.such", phase: "A" } as EntryPoint,
			],
		};
		const problems = coverageProblems(commands, stale, BUILD);
		expect(problems).toContain("gone.command is not in the registry");
		expect(problems).toContain("no.such is not an action");
	});

	test("the Prototype build requires only the commands marked P in section 9.1", () => {
		// The catalogue phase says when the service has a command; the UI phase
		// of the table says when the UI reaches it, so in the Prototype build the
		// rule covers the five commands of the table.
		const marked = new Set([
			"clip.add",
			"track.add",
			"item.split",
			"item.trim",
			"caption.add",
		]);
		const problems = coverageProblems(
			commands.filter((c) => marked.has(c.name)),
			Object.fromEntries(
				Object.entries(COVERAGE).filter(([n]) => marked.has(n)),
			),
			"prototype",
		);
		expect(problems).toEqual([]);
	});

	test("the builders emit the operations the actions declare", () => {
		const items = [{ id: "itm_a", start: 0, duration: 100 }];
		const newTrackId = (): string => "trk_a";
		const emitted: Record<string, string[]> = {
			"library.add_to_timeline": addClipOperations({
				assetId: "ast_a",
				playhead: 0,
				firstVisualTrack: null,
				newTrackId,
			}).map((o) => o.op),
			"timeline.split": splitOperations(items, 10).map((o) => o.op),
			"timeline.trim_start": trimOperations(items, "start", 10).map(
				(o) => o.op,
			),
			"timeline.trim_end": trimOperations(items, "end", 10).map((o) => o.op),
			"caption.add": captionOperations({
				text: "a",
				duration: "1s",
				playhead: 0,
				firstVisualTrack: null,
				newTrackId,
			}).map((o) => o.op),
		};
		for (const [action, ops] of Object.entries(emitted)) {
			expect(ops.length, action).toBeGreaterThan(0);
			for (const op of ops)
				expect(ACTIONS[action], `${action} sends ${op}`).toContain(op);
		}
		// Every action but the palette has a builder checked above.
		for (const action of Object.keys(ACTIONS)) {
			if (action !== "palette.run")
				expect(emitted[action], action).toBeDefined();
		}
	});
});

describe("isPhaseAtLeast", () => {
	test("orders prototype, alpha and beta", () => {
		expect(isPhaseAtLeast("alpha", "prototype")).toBe(false);
		expect(isPhaseAtLeast("alpha", "alpha")).toBe(true);
		expect(isPhaseAtLeast("alpha", "beta")).toBe(true);
		expect(isPhaseAtLeast("prototype", "prototype")).toBe(true);
	});
});
