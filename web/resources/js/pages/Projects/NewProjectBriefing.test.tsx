import { describe, expect, test } from "vitest";

import { drawForTest } from "../../workflow/drawForTest";
import { CHOICES } from "./kinds";
import NewProjectBriefing from "./NewProjectBriefing";
import ProjectContextForm from "./ProjectContextForm";
import { emptyContext } from "./projectSchema";

const NONE = {
	busy: false,
	failure: null,
	created: null,
	onEdited: () => undefined,
	onSubmit: () => undefined,
	onBack: () => undefined,
};

describe("NewProjectBriefing for each kind", () => {
	test("says what the briefing is for in the words of the kind, and none of them names another", () => {
		const words = {
			text: "what the text is about",
			video: "what the video is about",
			image: "what the image is for",
			undecided: "You choose the format after",
		} as const;
		for (const kind of CHOICES) {
			const html = drawForTest(<NewProjectBriefing {...NONE} kind={kind} />);
			expect(html, kind).toContain(words[kind]);
			expect(html, kind).not.toContain("ui.projects");
			for (const other of CHOICES) {
				if (other !== kind)
					expect(html, `${kind} vs ${other}`).not.toContain(words[other]);
			}
		}
	});

	test("shows the fields the service asks for and a back button, whatever the kind", () => {
		for (const kind of CHOICES) {
			const html = drawForTest(<NewProjectBriefing {...NONE} kind={kind} />);
			for (const label of [
				"Name",
				"Description",
				"Specific",
				"Measurable",
				"Achievable",
				"Relevant",
				"Time-bound",
				"Due date",
			]) {
				expect(html, `${kind} ${label}`).toContain(`>${label}</label>`);
			}
			expect(html).toContain("Create project");
			expect(html).toContain(">Back<");
		}
	});

	test("a text offers the example of a text and a video the example of a video, and nothing says video to a text", () => {
		const text = drawForTest(<NewProjectBriefing {...NONE} kind="text" />);
		expect(text).toContain("A blog post that explains");
		expect(text).not.toMatch(/video/i);
		const video = drawForTest(<NewProjectBriefing {...NONE} kind="video" />);
		expect(video).toContain("A 30 second vertical video");
		const later = drawForTest(
			<NewProjectBriefing {...NONE} kind="undecided" />,
		);
		expect(later).toContain("A short teaser");
		expect(later).not.toMatch(/video|footage/i);
	});

	test("leads to the project that exists when its first item was refused, with no form to send twice", () => {
		const html = drawForTest(
			<NewProjectBriefing
				{...NONE}
				kind="video"
				created="prj_0000000001"
				failure={{ code: "E_VALIDATION", message: "The format is not known." }}
			/>,
		);
		expect(html).toContain("The project was created");
		expect(html).toContain("The format is not known.");
		expect(html).toContain('href="/projects/prj_0000000001"');
		expect(html).not.toContain("Create project");
	});
});

describe("errors under each field", () => {
	const bad = {
		...emptyContext(),
		name: "abc",
		description: "too short",
		goal: {
			specific: "short",
			measurable: "short",
			achievable: "short",
			relevant: "short",
			time_bound: "short",
			due: "2001-01-01",
		},
	};

	test("every invalid field shows its error right after it, tied to it for a screen reader", () => {
		const html = drawForTest(
			<ProjectContextForm
				initial={bad}
				submitLabel="Create project"
				busy={false}
				keepDue=""
				onSubmit={() => undefined}
			/>,
		);
		const fields = [
			"name",
			"description",
			"specific",
			"measurable",
			"achievable",
			"relevant",
			"time_bound",
			"due",
		];
		for (const field of fields) {
			const input = new RegExp(
				`<(?:input|textarea)[^>]*name="${field}"[^>]*>`,
			).exec(html);
			expect(input, field).not.toBeNull();
			expect(input?.[0], field).toContain('aria-invalid="true"');
			const id = /id="([^"]+)"/.exec(input?.[0] ?? "")?.[1];
			expect(html, field).toContain(`aria-describedby="${id}-error"`);
			expect(html, field).toContain(`id="${id}-error"`);
		}
		expect(html.match(/role="alert"/g)).toHaveLength(fields.length);
		expect(html).toContain("Write at least 4 characters.");
		expect(html).toContain("Write at least 64 characters.");
		expect(html).toContain("Write at least 16 characters.");
		expect(html).toContain("The due date cannot be before today.");
		// The button waits while any of them is wrong.
		expect(html).toMatch(/<button[^>]*disabled=""[^>]*>Create project/);
	});

	test("a field the service refuses shows the refusal under that field", () => {
		const html = drawForTest(
			<ProjectContextForm
				initial={emptyContext()}
				submitLabel="Create project"
				busy={false}
				failure={{
					code: "E_VALIDATION",
					message: "The name is taken.",
					path: "name",
				}}
				onSubmit={() => undefined}
			/>,
		);
		const name = html.indexOf('name="name"');
		const refusal = html.indexOf("The name is taken.");
		const description = html.indexOf('name="description"');
		expect(refusal).toBeGreaterThan(name);
		expect(refusal).toBeLessThan(description);
	});
});
