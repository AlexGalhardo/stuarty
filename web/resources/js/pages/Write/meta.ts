import type {
	FlowStep,
	ProjectFlow,
	TextItem,
	TextKind,
	WriteProject,
} from "./client";

// What the editor says about a text is what the service said: the words of the text, and its minutes of reading
// when the service gave them. This file picks among the answers and counts nothing.

// The minutes to read a text: on the text itself when the service says so, otherwise from the summary of its
// project when the summary is about this very text. Absent when the service said neither.
export function readingMinutes(
	item: TextItem,
	project: WriteProject | undefined,
): number | undefined {
	if (item.reading_minutes !== undefined) return item.reading_minutes;
	const summary = project?.text_summary;
	return summary !== undefined && summary.item_id === item.id
		? summary.reading_minutes
		: undefined;
}

// A project that holds only text: no video or image action is offered in it (D-134). An undecided project is
// drawn as text too, since a person who opens Write has not chosen a medium; a project the service could not read
// is drawn the same, so nothing about a video is ever assumed.
export function isTextOnly(flow: ProjectFlow | null): boolean {
	return flow === null || flow.kind === "text" || flow.kind === "undecided";
}

// The steps of the path to the end of the text, as the service listed them, without the ones the person skipped.
export function pathSteps(flow: ProjectFlow | null): readonly FlowStep[] {
	return (flow?.steps ?? []).filter((step) => step.state !== "skipped");
}

// The step that follows the review in the list the service gave: the first one after it that is not done. With no
// review in the list it is the step the service calls next. The page walks the list and decides nothing.
export function stepAfterReview(
	flow: ProjectFlow | null,
): FlowStep | undefined {
	if (flow === null) return undefined;
	const steps = pathSteps(flow);
	const at = steps.findIndex((step) => step.id === "review");
	if (at < 0) return steps.find((step) => step.id === flow.next);
	return steps.slice(at + 1).find((step) => step.state !== "done");
}

// The one primary action of the editor, by where the person stands (SDD-019 section 11): unsaved text is saved
// first, saved text is reviewed next, and once reviewed the next step of the workflow is the primary one.
export type Primary = "save" | "review" | "next";

export function primaryAction(state: {
	dirty: boolean;
	reviewed: boolean;
}): Primary {
	if (state.dirty) return "save";
	return state.reviewed ? "next" : "review";
}

// How a text ends (SDD-019 section 11, D-137): a copy ends in Copy and Export with no destination; any other text
// goes on to the step of the path that follows its review, when the service lists one, and otherwise ends in Copy
// and Export too. The step is the service's: this only says which of the two is offered first.
export type End = { kind: "copy" } | { kind: "step"; step: FlowStep };

export function endOf(textKind: TextKind, flow: ProjectFlow | null): End {
	if (textKind === "copy") return { kind: "copy" };
	const step = stepAfterReview(flow);
	return step === undefined ? { kind: "copy" } : { kind: "step", step };
}

// The page a step is done in: the approvals, the publications or the project itself.
export function hrefOfStep(step: FlowStep, projectId: string): string {
	if (step.id === "approval") return "/approvals";
	if (step.id === "publication") return "/publish";
	return `/projects/${encodeURIComponent(projectId)}`;
}

// The name of the file an export of a text is saved as: the title with what a file name cannot hold taken out, and
// the id of the text when nothing is left. The body is the saved text, as the service stored it.
export function exportName(title: string, itemId: string): string {
	const base = title
		.normalize("NFKC")
		.replace(/[^\p{L}\p{N}]+/gu, "-")
		.replace(/^-+|-+$/g, "")
		.slice(0, 60)
		.replace(/-+$/g, "");
	return `${base === "" ? itemId : base}.md`;
}
