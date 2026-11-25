import { describe, expect, test } from "vitest";

import type { ProjectSummary } from "@/types";
import { drawForTest as draw } from "../../workflow/drawForTest";
import ProjectCard from "./ProjectCard";
import ProjectCardText from "./ProjectCardText";

const BASE: ProjectSummary = {
	id: "prj_0000000001",
	name: "Launch post",
	revision: 0,
	state: "active",
	width: 0,
	height: 0,
	fps: "30",
	duration: { frame: 0, seconds: 0 },
	tracks: 0,
	items: 0,
	assets: 0,
	timeline_id: "",
	timelines: 0,
	updated_at: "2026-11-24T10:00:00Z",
	kind: "text",
};

const NOOP = { onTrash: () => undefined, onChanged: () => undefined };

describe("what a card says of the plan", () => {
	test("names the step the project is at and the next post, on both cards", () => {
		const plan = {
			current_step: "review" as const,
			next_post_at: "2026-12-01T09:30:00Z",
		};
		const text = draw(
			<ProjectCardText {...NOOP} locale="en" project={{ ...BASE, ...plan }} />,
		);
		expect(text).toContain("Now: Review");
		expect(text).toContain("Next post");
		const video = draw(
			<ProjectCard
				{...NOOP}
				locale="en"
				project={{ ...BASE, kind: "video", ...plan }}
			/>,
		);
		expect(video).toContain("Now: Review");
		expect(video).toContain("Next post");
	});

	test("says nothing of a plan the service does not report", () => {
		const html = draw(
			<ProjectCardText
				{...NOOP}
				locale="en"
				project={{ ...BASE, next_post_at: null }}
			/>,
		);
		expect(html).not.toContain("Next post");
		expect(html).not.toContain("Now:");
	});
});

describe("ProjectCardText", () => {
	test("shows the excerpt, the type, the words and the reading time, and no picture", () => {
		const html = draw(
			<ProjectCardText
				{...NOOP}
				locale="en"
				project={{
					...BASE,
					text_summary: {
						item_id: "itm_1",
						text_kind: "post",
						words: 412,
						reading_minutes: 3,
						excerpt: "Today we are launching the first version",
					},
				}}
			/>,
		);
		expect(html).toContain("Today we are launching the first version");
		expect(html).toContain("Blog post");
		expect(html).toContain("412 words");
		expect(html).toContain("3 min read");
		expect(html).not.toContain("<img");
		// No empty thumbnail box, and no export of a bundle.
		expect(html).not.toContain("h-36");
		expect(html).not.toContain("Export");
		expect(html).toContain("/write?project=prj_0000000001&amp;text=itm_1");
	});

	test("a text with nothing written yet still looks complete", () => {
		const html = draw(<ProjectCardText {...NOOP} locale="en" project={BASE} />);
		expect(html).toContain("No text written yet.");
		expect(html).not.toContain("words");
		expect(html).toContain("/projects/prj_0000000001");
	});

	test("a singular word count reads in the singular", () => {
		const html = draw(
			<ProjectCardText
				{...NOOP}
				locale="en"
				project={{
					...BASE,
					text_summary: {
						item_id: "itm_1",
						text_kind: "caption",
						words: 1,
						reading_minutes: 1,
						excerpt: "Hi",
					},
				}}
			/>,
		);
		expect(html).toContain("1 word<");
	});
});

describe("ProjectCard", () => {
	test("a video has its picture box and its size", () => {
		const html = draw(
			<ProjectCard
				{...NOOP}
				locale="en"
				project={{
					...BASE,
					kind: "video",
					width: 1080,
					height: 1920,
					timelines: 1,
					items: 2,
					timeline_id: "tml_1",
				}}
			/>,
		);
		expect(html).toContain("h-36");
		expect(html).toContain("1080 × 1920");
		expect(html).toContain("/editor/tml_1");
	});

	test("a project not decided yet has no picture box and no size", () => {
		const html = draw(
			<ProjectCard
				{...NOOP}
				locale="en"
				project={{ ...BASE, kind: "undecided" }}
			/>,
		);
		expect(html).not.toContain("h-36");
		expect(html).toContain("Not decided yet");
		expect(html).not.toContain("0 × 0");
		expect(html).toContain("/projects/prj_0000000001");
	});
});
