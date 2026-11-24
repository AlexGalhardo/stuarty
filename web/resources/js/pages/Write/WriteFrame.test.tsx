import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { Frame } from "../../shell/testSupport";
import type { FlowStep, ProjectFlow, TextItem, WriteProject } from "./client";
import FlowPath from "./FlowPath";
import {
	isTextOnly,
	pathSteps,
	primaryAction,
	readingMinutes,
	stepAfterReview,
} from "./meta";

function step(
	id: FlowStep["id"],
	state: FlowStep["state"],
	over: Partial<FlowStep> = {},
): FlowStep {
	return { id, state, optional: false, actor: "both", ...over };
}

function item(over: Partial<TextItem> = {}): TextItem {
	return {
		id: "itm_1",
		project_id: "prj_1",
		title: "Post",
		text_kind: "post",
		state: "active",
		words: 400,
		revision: 2,
		updated_at: "2026-11-24T10:00:00Z",
		...over,
	};
}

const textFlow: ProjectFlow = {
	kind: "text",
	steps: [
		step("briefing", "done"),
		step("writing", "current", { actor: "agent" }),
		step("review", "upcoming", { actor: "person" }),
		step("approval", "upcoming", { actor: "person" }),
		step("publication", "upcoming", { optional: true }),
	],
	current: "writing",
	next: "review",
};

describe("the reading time of a text", () => {
	test("is the one the service put on the text", () => {
		expect(readingMinutes(item({ reading_minutes: 3 }), undefined)).toBe(3);
	});

	test("is the one of the project summary when it is about this text", () => {
		const project: WriteProject = {
			id: "prj_1",
			name: "P",
			text_summary: {
				item_id: "itm_1",
				text_kind: "post",
				words: 400,
				reading_minutes: 2,
			},
		};
		expect(readingMinutes(item(), project)).toBe(2);
	});

	test("is not invented when the summary is about another text", () => {
		const project: WriteProject = {
			id: "prj_1",
			name: "P",
			text_summary: {
				item_id: "other",
				text_kind: "post",
				words: 400,
				reading_minutes: 2,
			},
		};
		expect(readingMinutes(item(), project)).toBeUndefined();
		expect(readingMinutes(item(), undefined)).toBeUndefined();
	});
});

describe("the kind of the project", () => {
	test("a text, an undecided and an unread project are drawn as text", () => {
		expect(isTextOnly(null)).toBe(true);
		expect(isTextOnly({ kind: "text", steps: [] })).toBe(true);
		expect(isTextOnly({ kind: "undecided", steps: [] })).toBe(true);
	});

	test("a video project is not text only", () => {
		expect(isTextOnly({ kind: "video", steps: [] })).toBe(false);
	});
});

describe("the path after the review", () => {
	test("is the first step after the review that is not done", () => {
		expect(stepAfterReview(textFlow)?.id).toBe("approval");
		const moved: ProjectFlow = {
			...textFlow,
			steps: textFlow.steps.map((s) =>
				s.id === "approval" ? { ...s, state: "done" } : s,
			),
		};
		expect(stepAfterReview(moved)?.id).toBe("publication");
	});

	test("skipped steps are not part of the path", () => {
		const skipped: ProjectFlow = {
			...textFlow,
			steps: textFlow.steps.map((s) =>
				s.id === "approval" ? { ...s, state: "skipped" } : s,
			),
		};
		expect(pathSteps(skipped).map((s) => s.id)).not.toContain("approval");
		expect(stepAfterReview(skipped)?.id).toBe("publication");
	});

	test("with no review in the list it is the step the service calls next", () => {
		const flow: ProjectFlow = {
			kind: "image",
			steps: [step("briefing", "done"), step("editing", "current")],
			next: "editing",
		};
		expect(stepAfterReview(flow)?.id).toBe("editing");
		expect(stepAfterReview(null)).toBeUndefined();
	});
});

describe("the primary action", () => {
	test("is Save with unsaved text, then Review, then the next step", () => {
		expect(primaryAction({ dirty: true, reviewed: false })).toBe("save");
		expect(primaryAction({ dirty: true, reviewed: true })).toBe("save");
		expect(primaryAction({ dirty: false, reviewed: false })).toBe("review");
		expect(primaryAction({ dirty: false, reviewed: true })).toBe("next");
	});
});

describe("the path drawn", () => {
	test("lists the steps with who does them and where each stands", () => {
		const html = renderToString(
			<Frame>
				<FlowPath flow={textFlow} />
			</Frame>,
		);
		expect(html).toContain("Path of this project");
		expect(html).toContain("The agent does it");
		expect(html).toContain(">now<");
		expect(html).toContain("optional");
		expect(html).toContain('aria-current="step"');
	});

	test("a text path names no video step", () => {
		const html = renderToString(
			<Frame>
				<FlowPath flow={textFlow} />
			</Frame>,
		);
		for (const word of ["Record", "Edit<", "Finish", "Script"]) {
			expect(html).not.toContain(word);
		}
	});

	test("draws nothing when the service gave no path", () => {
		expect(
			renderToString(
				<Frame>
					<FlowPath flow={null} />
				</Frame>,
			),
		).toBe("");
	});
});
