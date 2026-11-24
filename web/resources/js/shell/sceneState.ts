import { openRequests } from "../dashboard/chat/chatState";
import type { ChatActivity, ChatEvent, ChatSession } from "../editor/api/types";

// How the scene bar tells a session (SDD-012 section 23.4). Every variant is a word and an icon in the
// component; this file only maps what the service reported to a variant. It adds no fact: the agent and the
// request are the service's, and so are the skill and the tool, which come from the activity read model of the
// service (SDD-017 section 8.1, epic 76). A next step and a cost are not drawn at all.
export type SceneVariant =
	| "none"
	| "idle"
	| "working"
	| "waiting"
	| "denied"
	| "done"
	| "failed";

export interface SceneTool {
	// untrusted text: the agent wrote it
	title: string;
	status: "pending" | "in_progress" | "completed" | "failed";
	// for a tool of Stuarty: its name and the message key of its plain description; null when only the agent's
	// own title is known
	technicalName: string | null;
	descriptionKey: string | null;
	fromAgent: boolean;
}

export interface SceneSkill {
	// untrusted text: the name of a skill of the library; empty unless the state is loaded
	name: string;
	// loaded: read in this session; available: a reviewed skill exists and none was read; none
	state: "loaded" | "available" | "none";
}

export interface SceneRequest {
	requestId: string;
	// untrusted text: the title of the tool that asks
	title: string;
}

export interface SceneModel {
	variant: SceneVariant;
	// the name of the profile that runs the session
	agent: string | null;
	// the state the session reports (ui.chat.state_*), for the word under the agent
	state: ChatSession["state"] | null;
	endReason: string | null;
	// whether the service has told the activity of this session: with no answer the bar says "not reported"
	// instead of guessing
	reported: boolean;
	skill: SceneSkill | null;
	tool: SceneTool | null;
	// the permission request that waits for a person, if any
	request: SceneRequest | null;
}

// A session that ended for a reason the person or Stuarty chose is done; any other reason is a failure.
const DONE_REASONS: ReadonlySet<string> = new Set(["user", "idle", "shutdown"]);

// The skill and the tool as the service reported them; nothing is derived from the events here.
export function reportedTool(activity: ChatActivity): SceneTool | null {
	const t = activity.tool;
	if (t === null) return null;
	const known = t.technical_name !== undefined && !t.from_agent;
	return {
		title: t.title,
		status: t.status,
		technicalName: known ? (t.technical_name ?? null) : null,
		descriptionKey: known ? (t.description_key ?? null) : null,
		fromAgent: !known,
	};
}

// The last answer of the turn in course, when it was a refusal. A turn starts at the last message of the
// person; the answer of a request of an earlier turn is not this turn's.
function refusedThisTurn(events: readonly ChatEvent[]): boolean {
	for (let i = events.length - 1; i >= 0; i--) {
		const e = events[i];
		if (e?.kind === "user_message") return false;
		if (e?.kind === "permission_answer") {
			return e.permission?.answer !== "allow";
		}
	}
	return false;
}

// The name of the project the session belongs to, as the project list holds it: the bar names it, because the
// session the shell reads can be of a project other than the page (the home, a section, the last one picked).
export function projectNameOf(
	session: Pick<ChatSession, "project_id"> | null,
	projects: readonly { id: string; name: string }[],
): string | null {
	if (session === null) return null;
	return projects.find((p) => p.id === session.project_id)?.name ?? null;
}

export function sceneOf(
	session: ChatSession | null,
	events: readonly ChatEvent[],
	activity: ChatActivity | null = null,
): SceneModel {
	if (session === null) {
		return {
			variant: "none",
			agent: null,
			state: null,
			endReason: null,
			reported: false,
			skill: null,
			tool: null,
			request: null,
		};
	}
	const open = openRequests(events)[0]?.permission;
	const request: SceneRequest | null =
		open === undefined
			? null
			: { requestId: open.request_id, title: open.title ?? "" };
	const base = {
		agent: session.profile,
		state: session.state,
		endReason: session.end_reason ?? null,
		reported: activity !== null,
		skill: activity?.skill ?? null,
		tool: activity === null ? null : reportedTool(activity),
		request,
	};
	switch (session.state) {
		case "ended":
			return {
				...base,
				request: null,
				variant:
					session.end_reason === undefined ||
					DONE_REASONS.has(session.end_reason)
						? "done"
						: "failed",
			};
		case "awaiting_permission":
			return { ...base, variant: "waiting" };
		case "running":
		case "starting":
			return { ...base, variant: "working" };
		default:
			return { ...base, variant: refusedThisTurn(events) ? "denied" : "idle" };
	}
}
