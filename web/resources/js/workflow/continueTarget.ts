import type { ProjectKind, StepId } from "./types";

// The page that goes on from the step the service says is current. It is a link, not a rule: which step is current
// is the service's word, and this only says where a person works on that kind of step. A text step opens the text
// when the project holds one (its item id), a media step opens the editor, and any other step opens the project.
export function continueTarget(
	step: StepId,
	projectId: string,
	textItemId?: string,
	projectKind?: ProjectKind,
): string {
	const id = encodeURIComponent(projectId);
	switch (step) {
		case "script":
		case "writing":
		case "review":
			return textItemId === undefined
				? `/projects/${id}`
				: `/write?project=${id}&text=${encodeURIComponent(textItemId)}`;
		case "recording":
		case "editing":
		case "finishing":
			return projectKind === "image" ? "/edit-image" : `/editor/${id}`;
		case "approval":
			return "/approvals";
		case "publication":
			return "/publish";
		default:
			return `/projects/${id}`;
	}
}

// The video editor is a separate HTML document with its own bundle (SDD-012 section 2.2), so visiting it through
// Inertia XHR would trigger Inertia's non-Inertia response overlay; callers must use a plain browser anchor for it.
export function isDocumentNavigation(href: string): boolean {
	return href.startsWith("/editor/");
}
