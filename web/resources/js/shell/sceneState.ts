import { openRequests } from "../dashboard/chat/chatState";
import type { ChatEvent, ChatSession } from "../editor/api/types";

// How the scene bar tells a session (SDD-012 section 23.4). Every variant is a word and an icon in the
// component; this file only maps what the service reported to a variant. It adds no fact: the agent, the
// tool and the request are the service's, and the skill, a next step and a cost are not drawn at all
// until the server reports them (epic 76).
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
	tool: SceneTool | null;
	// the permission request that waits for a person, if any
	request: SceneRequest | null;
}

// A session that ended for a reason the person or Stuarty chose is done; any other reason is a failure.
const DONE_REASONS: ReadonlySet<string> = new Set(["user", "idle", "shutdown"]);

export function lastTool(events: readonly ChatEvent[]): SceneTool | null {
	for (let i = events.length - 1; i >= 0; i--) {
		const e = events[i];
		if (e?.kind === "tool_call" && e.tool !== undefined) {
			return { title: e.tool.title, status: e.tool.status };
		}
	}
	return null;
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

export function sceneOf(
	session: ChatSession | null,
	events: readonly ChatEvent[],
): SceneModel {
	if (session === null) {
		return {
			variant: "none",
			agent: null,
			state: null,
			endReason: null,
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
		tool: lastTool(events),
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
