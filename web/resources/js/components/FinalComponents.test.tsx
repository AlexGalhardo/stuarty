import { FolderOpen } from "lucide-react";
import { describe, expect, test } from "vitest";

import { drawForTest } from "../workflow/drawForTest";
import { DetailPanel } from "./DetailPanel";
import { EmptyState } from "./EmptyState";
import { ErrorState } from "./ErrorState";
import { FilterBar } from "./FilterBar";
import { LoadError } from "./LoadError";
import { OptionSelect } from "./OptionSelect";
import { PageHeaderAction } from "./PageHeader";
import { Segment } from "./Segment";
import { TabBar } from "./TabBar";

const OPTIONS = [
	{ value: "all", label: "All" },
	{ value: "kept", label: "Kept" },
];

describe("the components of the final design", () => {
	test("the primary action of a header is 40 px high", () => {
		const html = drawForTest(<PageHeaderAction>New</PageHeaderAction>);
		expect(html).toContain('data-size="lg"');
		expect(html).toContain("h-10");
	});

	test("the select shows the label of the chosen option and is named", () => {
		const html = drawForTest(
			<OptionSelect
				value="kept"
				onChange={() => undefined}
				options={OPTIONS}
				label="Origin"
			/>,
		);
		expect(html).toContain('aria-label="Origin"');
		expect(html).toContain("Kept");
	});

	test("the segment marks the chosen option", () => {
		const html = drawForTest(
			<Segment
				value="kept"
				onChange={() => undefined}
				options={OPTIONS}
				label="View"
			/>,
		);
		expect(html).toContain('aria-label="View"');
		expect(html).toMatch(/aria-pressed="true"[^>]*>Kept|Kept.*aria-pressed/s);
	});

	test("the tab bar names its tabs and counts", () => {
		const html = drawForTest(
			<TabBar
				value="all"
				onChange={() => undefined}
				tabs={[{ ...OPTIONS[0]!, count: 3 }, OPTIONS[1]!]}
				label="Notes"
			/>,
		);
		expect(html).toContain('role="tablist"');
		expect(html).toContain("All");
		expect(html).toContain(">3<");
	});

	test("the filter bar is a labelled search region", () => {
		const html = drawForTest(<FilterBar label="Filters">x</FilterBar>);
		expect(html).toContain('aria-label="Filters"');
	});

	test("the detail panel says what to do when nothing is chosen", () => {
		const html = drawForTest(
			<DetailPanel title="Skill" empty="Choose one.">
				hidden body
			</DetailPanel>,
		);
		expect(html).toContain("Choose one.");
		expect(html).not.toContain("hidden body");
	});

	test("an empty state can carry up to three first steps", () => {
		const html = drawForTest(
			<EmptyState
				icon={FolderOpen}
				title="Empty"
				description="Nothing yet."
				steps={[{ title: "One" }, { title: "Two" }, { title: "Three" }]}
			/>,
		);
		expect(html.match(/data-slot="numbered-step"/g)).toHaveLength(3);
	});

	test("an error state offers Try again and How to fix, in Portuguese too", () => {
		const html = drawForTest(
			<ErrorState
				title="Failed"
				steps={["Do this"]}
				onRetry={() => undefined}
			/>,
			"pt-BR",
		);
		expect(html).toContain("Tentar de novo");
		expect(html).toContain("Como resolver");
		expect(html).toContain('role="alert"');
	});

	test("a failed read never shows the raw message of the server", () => {
		const html = drawForTest(
			<LoadError
				failure={{ code: "E_NETWORK", message: "network failure" }}
				onRetry={() => undefined}
			/>,
			"pt-BR",
		);
		expect(html).not.toContain("network failure");
		expect(html).toContain("Não consegui falar com o Stuarty");
		expect(html).toContain("Tentar de novo");
	});
});
