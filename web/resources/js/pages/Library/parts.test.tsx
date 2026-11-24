import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import en from "../../../lang/en.json";
import type { LibraryEntry } from "./client";
import { EntryBadges, Findings, groupEntries } from "./parts";

function flat(
	tree: Record<string, unknown>,
	prefix: string,
	out: Record<string, string> = {},
): Record<string, string> {
	for (const [key, value] of Object.entries(tree)) {
		if (typeof value === "string") out[`${prefix}${key}`] = value;
		else flat(value as Record<string, unknown>, `${prefix}${key}.`, out);
	}
	return out;
}
const messages = flat(en.ui, "ui.");

const draw = (node: React.ReactNode): string =>
	renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
			{node}
		</I18nProvider>,
	);

const base: LibraryEntry = {
	kind: "skill",
	name: "hook",
	description: "d",
	origin: "imported",
	reviewed: false,
	kept: false,
	state: "ok",
	updated_at: "2026-11-24T12:00:00Z",
};

describe("EntryBadges says in words whether an agent is offered the entry", () => {
	test("a skill nobody read says that it is not offered to agents", () => {
		const html = draw(<EntryBadges entry={base} />);
		expect(html).toContain("Not reviewed: not offered to agents");
		expect(html).toContain("Imported");
	});

	test("a reviewed skill says that it is offered", () => {
		const html = draw(<EntryBadges entry={{ ...base, reviewed: true }} />);
		expect(html).toContain("Reviewed: offered to agents");
		expect(html).not.toContain("not offered");
	});

	test("a pending note says that it waits for a person, a kept one that it is offered", () => {
		const note: LibraryEntry = { ...base, kind: "memory", origin: undefined };
		expect(draw(<EntryBadges entry={note} />)).toContain(
			"Waiting for you: not offered to agents",
		);
		expect(draw(<EntryBadges entry={{ ...note, kept: true }} />)).toContain(
			"Kept: offered to agents",
		);
	});

	test("an invalid file is not offered, and the state is a word as well as an icon", () => {
		const html = draw(<EntryBadges entry={{ ...base, state: "invalid" }} />);
		expect(html).toContain("Invalid file: not offered to agents");
		expect(html).toContain('data-slot="status-badge"');
	});
});

describe("groupEntries puts the notes that wait for a person first", () => {
	const note = (name: string, kept: boolean): LibraryEntry => ({
		...base,
		kind: "memory",
		name,
		kept,
	});
	const labels = { pending: "Waiting", kept: "Kept", invalid: "Unreadable" };

	test("pending notes come before the kept ones, each group with its heading", () => {
		const groups = groupEntries(
			"memory",
			[note("a", true), note("b", false), note("c", true)],
			labels,
		);
		expect(groups.map((g) => g.key)).toEqual(["pending", "kept"]);
		expect(groups[0]?.entries.map((e) => e.name)).toEqual(["b"]);
		expect(groups[1]?.entries.map((e) => e.name)).toEqual(["a", "c"]);
	});

	test("a note that cannot be read is never listed under Kept", () => {
		const broken: LibraryEntry = { ...note("x", false), state: "invalid" };
		const groups = groupEntries("memory", [note("a", true), broken], labels);
		expect(groups.map((g) => g.key)).toEqual(["kept", "invalid"]);
		expect(groups[0]?.entries.map((e) => e.name)).toEqual(["a"]);
		expect(groups[1]?.entries.map((e) => e.name)).toEqual(["x"]);
	});

	test("a group with no entry is not listed", () => {
		const groups = groupEntries("memory", [note("a", true)], labels);
		expect(groups.map((g) => g.key)).toEqual(["kept"]);
	});

	test("skills and prompts keep the order of the service, with no heading", () => {
		const groups = groupEntries("skill", [base], labels);
		expect(groups).toHaveLength(1);
		expect(groups[0]?.label).toBeNull();
	});
});

describe("Findings draws text from a file as text", () => {
	test("markup in a finding is shown, never made an element", () => {
		const html = draw(
			<Findings
				findings={[
					{
						code: "L1",
						severity: "warning",
						path: "<img src=x onerror=1>",
						message: "<script>1</script>",
					},
				]}
			/>,
		);
		expect(html).not.toContain("<img");
		expect(html).not.toContain("<script");
		expect(html).toContain("&lt;img");
	});
});
