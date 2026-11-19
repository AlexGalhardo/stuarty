import { useCallback, useEffect, useRef, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	AgentProfileListResult,
	ChatEvent,
	ChatSession,
} from "../../editor/api/types";
import { applyChatEvent, applySnapshot, stateOf } from "./chatState";

export interface ChatPanelState {
	profiles: AgentProfileListResult | null;
	projects: ProjectSummary[];
	projectId: string | null;
	session: ChatSession | null;
	events: ChatEvent[];
	// the stream of the session is open: what the agent says arrives by itself
	live: boolean;
	failure: Failure | null;
	busy: boolean;
	selectProject(id: string): void;
	start(profile: string): Promise<void>;
	send(text: string): Promise<void>;
	cancel(end: boolean): Promise<void>;
	answer(
		requestId: string,
		answer: "allow" | "reject" | "cancel",
	): Promise<void>;
	exportMarkdown(): Promise<void>;
	removeSession(): Promise<void>;
	saveProfile(name: string, command: string, args: string[]): Promise<boolean>;
	removeProfile(name: string): Promise<void>;
}

const STORED_PROJECT = "stuarty.chat.project";

function parseFrame(data: string): ChatEvent | null {
	try {
		const raw: unknown = JSON.parse(data);
		if (
			typeof raw === "object" &&
			raw !== null &&
			typeof (raw as { seq?: unknown }).seq === "number" &&
			typeof (raw as { kind?: unknown }).kind === "string"
		) {
			return raw as ChatEvent;
		}
	} catch {
		// a frame that is not JSON is dropped
	}
	return null;
}

// The state of the chat panel: the profiles, the project, the session and its events. It shows
// what the service says and sends the person's commands; nothing here decides what the agent may
// do (SDD-017). The events of a live session come over the stream, which opens with everything
// said so far, so a reconnect cannot leave a hole or repeat a line.
export function useChat(
	active: boolean,
	initialProject: string | null,
): ChatPanelState {
	const [profiles, setProfiles] = useState<AgentProfileListResult | null>(null);
	const [projects, setProjects] = useState<ProjectSummary[]>([]);
	const [projectId, setProjectId] = useState<string | null>(initialProject);
	const [session, setSession] = useState<ChatSession | null>(null);
	const [events, setEvents] = useState<ChatEvent[]>([]);
	const [live, setLive] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const transport = useRef<HttpTransport | null>(null);
	if (transport.current === null) {
		transport.current = new HttpTransport("/api/v1");
	}
	const stream = useRef<{ close(): void } | null>(null);
	// counts what the person did to the session (start, delete): an answer to a list that was asked
	// before is older than that and must not undo it
	const actions = useRef(0);

	const api = useCallback(() => {
		if (transport.current === null) throw new Error("no transport");
		return createClient(transport.current);
	}, []);

	const closeStream = useCallback(() => {
		stream.current?.close();
		stream.current = null;
		setLive(false);
	}, []);

	const guard = useCallback(
		async <T>(work: () => Promise<T>): Promise<T | undefined> => {
			setFailure(null);
			setBusy(true);
			try {
				return await work();
			} catch (error) {
				setFailure(failureOf(error));
				return undefined;
			} finally {
				setBusy(false);
			}
		},
		[],
	);

	const refreshSession = useCallback(
		async (id: string): Promise<void> => {
			try {
				const res = await api().listChatSessions(id);
				setSession(res.sessions[0] ?? null);
			} catch (error) {
				setFailure(failureOf(error));
			}
		},
		[api],
	);

	const open = useCallback(
		(s: ChatSession) => {
			closeStream();
			setEvents([]);
			const http = transport.current;
			if (http === null) return;
			if (!s.live) {
				// an ended session is read, not followed
				api()
					.getChatTranscript(s.id)
					.then((res) =>
						setEvents((was) => applySnapshot(was, res.events ?? [])),
					)
					.catch((error: unknown) => setFailure(failureOf(error)));
				return;
			}
			stream.current = http.openStream(`/chat/sessions/${s.id}/events`, {
				onOpen: () => setLive(true),
				onMessage: (name, data) => {
					if (name !== "chat.event") return;
					const frame = parseFrame(data);
					if (frame === null) return;
					setEvents((was) => applyChatEvent(was, frame));
					if (frame.kind === "state" && frame.state?.state === "ended") {
						closeStream();
						void refreshSession(s.project_id);
					}
				},
				onError: () => setLive(false),
			});
		},
		[api, closeStream, refreshSession],
	);

	// profiles and projects, read when the panel opens
	useEffect(() => {
		if (!active) return;
		let alive = true;
		api()
			.listAgentProfiles()
			.then((res) => alive && setProfiles(res))
			.catch((error: unknown) => alive && setFailure(failureOf(error)));
		api()
			.listProjects()
			.then((res) => {
				if (!alive) return;
				setProjects(res.projects);
				setProjectId((was) => {
					if (was !== null && res.projects.some((p) => p.id === was))
						return was;
					const stored = window.localStorage.getItem(STORED_PROJECT);
					if (stored !== null && res.projects.some((p) => p.id === stored)) {
						return stored;
					}
					return res.projects[0]?.id ?? null;
				});
			})
			.catch((error: unknown) => alive && setFailure(failureOf(error)));
		return () => {
			alive = false;
		};
	}, [active, api]);

	// the session of the project: the newest one, followed when it is live
	useEffect(() => {
		if (!active || projectId === null) return;
		let alive = true;
		const seen = actions.current;
		api()
			.listChatSessions(projectId)
			.then((res) => {
				if (!alive || actions.current !== seen) return;
				const latest = res.sessions[0] ?? null;
				setSession(latest);
				if (latest === null) {
					closeStream();
					setEvents([]);
				} else {
					open(latest);
				}
			})
			.catch((error: unknown) => alive && setFailure(failureOf(error)));
		return () => {
			alive = false;
		};
	}, [active, projectId, api, open, closeStream]);

	useEffect(() => closeStream, [closeStream]);

	const selectProject = useCallback((id: string) => {
		window.localStorage.setItem(STORED_PROJECT, id);
		setProjectId(id);
	}, []);

	const start = useCallback(
		async (profile: string): Promise<void> => {
			if (projectId === null) return;
			actions.current++;
			const s = await guard(() => api().startChatSession(projectId, profile));
			if (s !== undefined) {
				setSession(s);
				open(s);
			}
		},
		[api, guard, open, projectId],
	);

	const send = useCallback(
		async (text: string): Promise<void> => {
			if (session === null) return;
			const s = await guard(() => api().sendChatMessage(session.id, text));
			if (s !== undefined) setSession(s);
		},
		[api, guard, session],
	);

	const cancel = useCallback(
		async (end: boolean): Promise<void> => {
			if (session === null) return;
			const s = await guard(() => api().cancelChat(session.id, end));
			if (s !== undefined) setSession(s);
		},
		[api, guard, session],
	);

	const answer = useCallback(
		async (
			requestId: string,
			choice: "allow" | "reject" | "cancel",
		): Promise<void> => {
			if (session === null) return;
			await guard(() =>
				api().answerChatPermission(session.id, requestId, choice),
			);
		},
		[api, guard, session],
	);

	const exportMarkdown = useCallback(async (): Promise<void> => {
		if (session === null) return;
		const res = await guard(() =>
			api().getChatTranscript(session.id, { format: "markdown" }),
		);
		if (res?.markdown === undefined) return;
		const url = URL.createObjectURL(
			new Blob([res.markdown], { type: "text/markdown" }),
		);
		const link = document.createElement("a");
		link.href = url;
		link.download = `chat-${session.id}.md`;
		link.click();
		URL.revokeObjectURL(url);
	}, [api, guard, session]);

	const removeSession = useCallback(async (): Promise<void> => {
		if (session === null) return;
		actions.current++;
		const removed = await guard(() => api().deleteChatSession(session.id));
		if (removed !== undefined) {
			closeStream();
			setSession(null);
			setEvents([]);
		}
	}, [api, closeStream, guard, session]);

	const reloadProfiles = useCallback(async (): Promise<void> => {
		setProfiles(await api().listAgentProfiles());
	}, [api]);

	const saveProfile = useCallback(
		async (name: string, command: string, args: string[]): Promise<boolean> => {
			const saved = await guard(async () => {
				const p = await api().setAgentProfile(name, command, args);
				await reloadProfiles();
				return p;
			});
			return saved !== undefined;
		},
		[api, guard, reloadProfiles],
	);

	const removeProfile = useCallback(
		async (name: string): Promise<void> => {
			await guard(async () => {
				await api().removeAgentProfile(name);
				await reloadProfiles();
			});
		},
		[api, guard, reloadProfiles],
	);

	// the state of the session as the events say it, ahead of the next refresh of the row
	const shown =
		session === null
			? null
			: { ...session, state: stateOf(events) ?? session.state };

	return {
		profiles,
		projects,
		projectId,
		session: shown,
		events,
		live,
		failure,
		busy,
		selectProject,
		start,
		send,
		cancel,
		answer,
		exportMarkdown,
		removeSession,
		saveProfile,
		removeProfile,
	};
}
