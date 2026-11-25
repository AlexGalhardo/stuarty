import type { StepId } from "./types";

// The page that goes on from the step the service says is current. It is a link, not a rule: which step is current
// is the service's word, and this only says where a person works on that kind of step. A text step opens the text
// when the project holds one (its item id), a media step opens the editor, and any other step opens the project.
export function continueTarget(
	step: StepId,
	projectId: string,
	textItemId?: string,
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
			return `/editor/${id}`;
		case "approval":
			return "/approvals";
		case "publication":
			return "/publish";
		default:
			return `/projects/${id}`;
	}
}
