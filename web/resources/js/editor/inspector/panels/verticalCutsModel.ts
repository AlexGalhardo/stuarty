import type { ApiError } from "../../api/errors";
import type {
	CutsProposal,
	ToolOffer,
	ToolsStatusResult,
} from "../../api/types";

// The name `tools:install` and the install route take for the face tracking set (SDD-024 section 3).
export const FACE_SET = "face";

// The longest title of a project item (SDD-005): 1 to 120 characters.
const MAX_TITLE = 120;

// The offer of the face set, with what it costs and its licences, or undefined when this build has none.
export function faceOffer(status: ToolsStatusResult): ToolOffer | undefined {
	return status.offers?.find((offer) => offer.name === FACE_SET);
}

// The face set is the thing the service says is missing: E_TRANSCRIBE_UNAVAILABLE names its tool.
export function isFaceUnavailable(error: {
	code: string;
	details?: ApiError["details"];
}): boolean {
	return (
		error.code === "E_TRANSCRIBE_UNAVAILABLE" &&
		error.details?.tool === FACE_SET
	);
}

// m:ss of a time in milliseconds; a label only, the service decides every range.
export function clockLabel(ms: number): string {
	const total = Math.floor(ms / 1000);
	const seconds = total % 60;
	return `${Math.floor(total / 60)}:${seconds < 10 ? "0" : ""}${seconds}`;
}

// The share of the range with a face on screen, in thousandths, as a whole percentage.
export function sharePercent(thousandths: number): number {
	return Math.round(thousandths / 10);
}

// The title of the timeline a proposal becomes: its rank and its first words, cut to what an item allows.
export function timelineTitle(proposal: CutsProposal): string {
	const text = `Cut ${proposal.n}: ${proposal.title}`.trim();
	return text.length <= MAX_TITLE ? text : `${text.slice(0, MAX_TITLE - 1)}…`;
}
