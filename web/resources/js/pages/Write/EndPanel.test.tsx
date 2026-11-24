import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { Frame } from "../../shell/testSupport";
import type { FlowStep, ProjectFlow, TextKind } from "./client";
import EndPanel, { type EndStatus } from "./EndPanel";
import { type End, endOf, exportName, hrefOfStep } from "./meta";

function step(id: FlowStep["id"], state: FlowStep["state"]): FlowStep {
	return { id, state, optional: false, actor: "person" };
}

const flow: ProjectFlow = {
	kind: "text",
	steps: [
		step("briefing", "done"),
		step("writing", "done"),
		step("review", "current"),
		step("approval", "upcoming"),
		step("publication", "upcoming"),
	],
	current: "review",
	next: "approval",
};

function draw(
	kind: TextKind,
	over: {
		end?: End;
		dirty?: boolean;
		reviewed?: boolean;
		status?: EndStatus | null;
	} = {},
): string {
	return renderToString(
		<Frame>
			<EndPanel
				kind={kind}
				end={over.end ?? endOf(kind, flow)}
				projectId="prj_1"
				dirty={over.dirty ?? false}
				reviewed={over.reviewed ?? false}
				status={over.status ?? null}
				onCopy={() => {}}
				onExport={() => {}}
			/>
		</Frame>,
	);
}

describe("how a text ends", () => {
	test("a copy ends in Copy and Export, whatever the path lists after the review", () => {
		expect(endOf("copy", flow)).toEqual({ kind: "copy" });
	});

	test("a post goes on to the step after its review", () => {
		const end = endOf("post", flow);
		expect(end.kind).toBe("step");
		if (end.kind === "step") expect(end.step.id).toBe("approval");
	});

	test("a text with no step after its review ends in Copy and Export", () => {
		expect(endOf("post", null)).toEqual({ kind: "copy" });
		const done: ProjectFlow = {
			...flow,
			steps: flow.steps.map((s) => ({ ...s, state: "done" })),
		};
		expect(endOf("script", done)).toEqual({ kind: "copy" });
	});

	test("a step has its page", () => {
		expect(hrefOfStep(step("approval", "upcoming"), "prj_1")).toBe(
			"/approvals",
		);
		expect(hrefOfStep(step("publication", "upcoming"), "prj_1")).toBe(
			"/publish",
		);
		expect(hrefOfStep(step("editing", "upcoming"), "prj 1")).toBe(
			"/projects/prj%201",
		);
	});
});

describe("the file of an export", () => {
	test("is the title without what a name cannot hold", () => {
		expect(exportName("My first post: draft/2", "itm_1")).toBe(
			"My-first-post-draft-2.md",
		);
		expect(exportName("Título com acento", "itm_1")).toBe(
			"Título-com-acento.md",
		);
	});

	test("falls back to the id when nothing is left", () => {
		expect(exportName("???", "itm_9")).toBe("itm_9.md");
		expect(exportName("../../etc/passwd", "itm_9")).toBe("etc-passwd.md");
	});
});

describe("the end panel of a copy", () => {
	test("offers Copy and Export and says no destination is needed", () => {
		const html = draw("copy");
		expect(html).toContain(">Copy<");
		expect(html).toContain(">Export<");
		expect(html).toContain("need no connection and no destination");
		expect(html).not.toContain("Next:");
	});

	test("sending it to a blog is a choice that starts closed", () => {
		const html = draw("copy");
		expect(html).toContain("Send it to a blog");
		expect(html).toContain('aria-expanded="false"');
		expect(html).not.toContain('id="end-blog"');
		expect(html).not.toContain("Nothing is sent from here");
		expect(html).not.toContain("/publish");
	});

	test("Copy is the primary action once the text is reviewed and saved", () => {
		expect(draw("copy", { reviewed: true })).toContain('data-primary="true"');
		expect(draw("copy", { reviewed: false })).not.toContain(
			'data-primary="true"',
		);
		expect(draw("copy", { reviewed: true, dirty: true })).not.toContain(
			'data-primary="true"',
		);
	});

	test("with unsaved changes Copy and Export wait and the panel says why", () => {
		const html = draw("copy", { dirty: true });
		expect(html).toContain("Save your changes first");
		expect(html).toContain('disabled=""');
		expect(draw("copy")).not.toContain('disabled=""');
	});

	test("says what was done", () => {
		expect(draw("copy", { status: { state: "copied" } })).toContain(
			"The saved text is on your clipboard.",
		);
		expect(
			draw("copy", { status: { state: "exported", file: "a.md" } }),
		).toContain("Exported as a.md.");
		expect(draw("copy", { status: { state: "failed" } })).toContain(
			"could not be copied",
		);
	});
});

describe("the end panel of a post", () => {
	test("offers the next step of the path and no blog choice of its own", () => {
		const html = draw("post", { reviewed: true });
		expect(html).toContain("Next: Approve");
		expect(html).toContain('href="/approvals"');
		expect(html).toContain('data-primary="true"');
		expect(html).not.toContain("Send it to a blog");
	});

	test("names no video step", () => {
		const html = draw("post");
		for (const word of ["Record", "Cut", "Render"]) {
			expect(html).not.toContain(word);
		}
	});
});
