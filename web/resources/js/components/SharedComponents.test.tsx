import { FolderOpen } from "lucide-react";
import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { DataList, DataRow } from "./DataRow";
import { EmptyState } from "./EmptyState";
import { GuideStrip } from "./GuideStrip";
import { ListRow } from "./ListRow";
import { LoadingState } from "./LoadingState";
import { NumberedStep } from "./NumberedStep";
import { PageHeader } from "./PageHeader";
import { SectionTitle } from "./SectionTitle";
import { type Status, StatusBadge } from "./StatusBadge";

describe("PageHeader", () => {
	test("names the page with its one h1", () => {
		const html = renderToString(
			<PageHeader title="Projects" description="Everything you make." />,
		);
		expect(html).toMatch(/<h1[^>]*>Projects<\/h1>/);
		expect(html).toContain("Everything you make.");
		expect(html.match(/<h1/g)).toHaveLength(1);
	});

	test("draws the actions only when given", () => {
		expect(renderToString(<PageHeader title="Projects" />)).not.toContain(
			"<button",
		);
		expect(
			renderToString(
				<PageHeader
					title="Projects"
					actions={<button type="button">New</button>}
				/>,
			),
		).toContain("<button");
	});
});

describe("SectionTitle", () => {
	test("is an h2 by default and an h3 on request", () => {
		expect(renderToString(<SectionTitle title="Assets" />)).toMatch(
			/<h2[^>]*>Assets<\/h2>/,
		);
		expect(renderToString(<SectionTitle title="Assets" level={3} />)).toMatch(
			/<h3[^>]*>Assets<\/h3>/,
		);
	});
});

describe("EmptyState", () => {
	test("is a region named by its title", () => {
		const html = renderToString(
			<EmptyState
				icon={FolderOpen}
				title="No projects yet"
				description="Create the first one."
			/>,
		);
		const labelledBy = html.match(/aria-labelledby="([^"]+)"/)?.[1];
		expect(labelledBy).toBeTruthy();
		expect(html).toContain(`id="${labelledBy}"`);
		expect(html).toContain("No projects yet");
		expect(html).toContain('aria-hidden="true"');
	});

	test("draws its actions", () => {
		expect(
			renderToString(
				<EmptyState icon={FolderOpen} title="T" description="D">
					<button type="button">Create</button>
				</EmptyState>,
			),
		).toContain("Create");
	});
});

describe("GuideStrip and NumberedStep", () => {
	test("is a labelled region holding an ordered list", () => {
		const html = renderToString(
			<GuideStrip title="How it works">
				<NumberedStep number={1} title="Brief" />
				<NumberedStep number={2} title="Write" />
			</GuideStrip>,
		);
		expect(html).toContain('aria-label="How it works"');
		expect(html).toContain("<ol");
		expect(html.match(/<li/g)).toHaveLength(2);
	});

	test("marks the current step and never relies on colour alone", () => {
		const current = renderToString(
			<ul>
				<NumberedStep
					number={2}
					title="Write"
					state="current"
					stateLabel="Current"
				/>
			</ul>,
		);
		expect(current).toContain('aria-current="step"');
		expect(current).toContain("Current");
		expect(current).toContain(">2<");
	});

	test("shows a check and the word for a done step, and a number for a todo one", () => {
		const done = renderToString(
			<ul>
				<NumberedStep number={1} title="Brief" state="done" stateLabel="Done" />
			</ul>,
		);
		expect(done).toContain("Done");
		expect(done).toContain("<svg");
		expect(done).not.toContain("aria-current");
		const todo = renderToString(
			<ul>
				<NumberedStep number={3} title="Publish" />
			</ul>,
		);
		expect(todo).toContain(">3<");
		expect(todo).not.toContain("<svg");
	});
});

describe("DataRow", () => {
	test("pairs a name with its value in a description list", () => {
		const html = renderToString(
			<DataList>
				<DataRow label="Format" hint="Set at creation">
					9:16
				</DataRow>
			</DataList>,
		);
		expect(html).toContain("<dl");
		expect(html).toMatch(/<dt[^>]*>Format<\/dt>/);
		expect(html).toMatch(/<dd[^>]*>9:16<\/dd>/);
		expect(html).toContain("Set at creation");
	});
});

describe("ListRow", () => {
	test("shows the title, the sentence and the meta line", () => {
		const html = renderToString(
			<ul>
				<ListRow title="Intro" description="Opening shot" meta="0:12" />
			</ul>,
		);
		expect(html).toContain("Intro");
		expect(html).toContain("Opening shot");
		expect(html).toContain("0:12");
		expect(html).not.toContain("aria-current");
	});

	test("the selected row is current and carries its own marker", () => {
		const html = renderToString(
			<ul>
				<ListRow title="Intro" selected />
			</ul>,
		);
		expect(html).toContain('aria-current="true"');
		expect(html).toContain('data-selected="true"');
		expect(html).toContain("border-l-2");
	});
});

describe("StatusBadge", () => {
	const statuses: Status[] = [
		"neutral",
		"info",
		"agent",
		"waiting",
		"success",
		"danger",
	];

	test.each(statuses)(
		"%s draws an icon hidden from readers and the word",
		(status) => {
			const html = renderToString(
				<StatusBadge status={status}>Word</StatusBadge>,
			);
			expect(html).toContain("<svg");
			expect(html).toContain('aria-hidden="true"');
			expect(html).toContain(`data-status="${status}"`);
			expect(html.replace(/<svg[\s\S]*?<\/svg>/, "")).toContain("Word");
		},
	);
});

describe("LoadingState", () => {
	test("is a polite status with a sentence and a static skeleton", () => {
		const html = renderToString(<LoadingState message="Loading projects" />);
		expect(html).toContain('role="status"');
		expect(html).toContain("Loading projects");
		expect(html).not.toContain("animate-");
		expect(html.match(/data-slot="skeleton"/g)).toHaveLength(3);
	});
});
