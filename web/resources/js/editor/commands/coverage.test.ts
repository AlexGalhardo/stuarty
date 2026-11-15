import { describe, expect, test } from "vitest";

import registry from "@/generated/commands.json";
import {
	addClipOperations,
	captionOperations,
	splitOperations,
	trimOperations,
} from "./builders";
import { ACTIONS, COVERAGE } from "./coverage";

interface RegistryEntry {
	name: string;
	internal: boolean;
	phase: string;
}

const commands = registry as RegistryEntry[];
const byName = new Map(commands.map((c) => [c.name, c]));

// The commands marked P in the table of SDD-012 section 9.1.
const PROTOTYPE_UI_COMMANDS = [
	"clip.add",
	"track.add",
	"item.split",
	"item.trim",
	"caption.add",
];

describe("command coverage (SDD-012 section 9.3)", () => {
	test("every command the Prototype UI sends has an entry point of phase P", () => {
		for (const name of PROTOTYPE_UI_COMMANDS) {
			const entries = COVERAGE[name] ?? [];
			expect(entries.length, name).toBeGreaterThan(0);
			expect(
				entries.some((e) => e.phase === "P"),
				name,
			).toBe(true);
		}
	});

	test("every key of COVERAGE is a command of the registry", () => {
		for (const name of Object.keys(COVERAGE)) {
			expect(byName.has(name), `${name} is not in the registry`).toBe(true);
			expect(byName.get(name)?.internal, name).toBe(false);
		}
	});

	test("every entry point names an action of the registry that sends the command", () => {
		for (const [name, entries] of Object.entries(COVERAGE)) {
			for (const entry of entries) {
				expect(
					ACTIONS[entry.action],
					`${entry.action} is not an action`,
				).toBeDefined();
				expect(ACTIONS[entry.action], `${entry.action} for ${name}`).toContain(
					name,
				);
			}
		}
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
	});
});
