import {
	type FormEvent,
	type KeyboardEvent,
	type ReactNode,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";

import { FailureNote } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import ChatEventView from "./ChatEventView";
import { canSend, isBusy, isVisible, openRequests } from "./chatState";
import type { ChatPanelState } from "./useChat";

const SELECT =
	"h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50";

// The conversation: the project and the agent, what was said, and the box to say more. It draws
// what the service says; whether a message may be sent is the state the server reports.
export default function ChatConversation({
	chat,
	composerRef,
}: {
	chat: ChatPanelState;
	composerRef: React.RefObject<HTMLTextAreaElement | null>;
}): ReactNode {
	const t = useT();
	const ids = useId();
	const [text, setText] = useState("");
	const [profile, setProfile] = useState("");
	const [confirmDelete, setConfirmDelete] = useState(false);
	const log = useRef<HTMLOListElement | null>(null);
	const stuck = useRef(true);

	const { session, events, profiles } = chat;
	const state = session?.state ?? null;
	const waiting = new Set(
		openRequests(events).map((e) => e.permission?.request_id ?? ""),
	);
	const names = profiles?.profiles.map((p) => p.name) ?? [];
	const chosen =
		profile !== "" && names.includes(profile) ? profile : (names[0] ?? "");
	const live = session?.live === true;

	// follow the conversation while the person is at the bottom of it
	// biome-ignore lint/correctness/useExhaustiveDependencies: scroll after every new event
	useEffect(() => {
		const el = log.current;
		if (el !== null && stuck.current) el.scrollTop = el.scrollHeight;
	}, [events]);

	const submit = async (event: FormEvent): Promise<void> => {
		event.preventDefault();
		const message = text.trim();
		if (message === "" || !canSend(state) || chat.busy) return;
		setText("");
		await chat.send(message);
	};

	const onKey = (event: KeyboardEvent<HTMLTextAreaElement>): void => {
		if (
			event.key === "Enter" &&
			!event.shiftKey &&
			!event.nativeEvent.isComposing
		) {
			void submit(event);
		}
	};

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-3">
			<div className="grid grid-cols-2 gap-2">
				<div className="flex flex-col gap-1">
					<Label htmlFor={`${ids}-project`} className="text-xs">
						{t("ui.chat.project")}
					</Label>
					<select
						id={`${ids}-project`}
						className={SELECT}
						value={chat.projectId ?? ""}
						onChange={(event) => chat.selectProject(event.target.value)}
					>
						{chat.projects.map((p) => (
							<option key={p.id} value={p.id}>
								{p.name}
							</option>
						))}
					</select>
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor={`${ids}-agent`} className="text-xs">
						{t("ui.chat.agent_label")}
					</Label>
					<select
						id={`${ids}-agent`}
						className={SELECT}
						value={chosen}
						onChange={(event) => setProfile(event.target.value)}
					>
						{names.map((n) => (
							<option key={n} value={n}>
								{n}
							</option>
						))}
					</select>
				</div>
			</div>
			<div className="flex flex-wrap items-center gap-2">
				<Button
					size="sm"
					disabled={chat.busy || chat.projectId === null || chosen === ""}
					onClick={() => void chat.start(chosen)}
				>
					{live ? t("ui.chat.new_session") : t("ui.chat.start")}
				</Button>
				{isBusy(state) && (
					<Button
						size="sm"
						variant="outline"
						disabled={chat.busy}
						onClick={() => void chat.cancel(false)}
					>
						{t("ui.chat.stop")}
					</Button>
				)}
				{live && (
					<Button
						size="sm"
						variant="ghost"
						disabled={chat.busy}
						onClick={() => void chat.cancel(true)}
					>
						{t("ui.chat.end_session")}
					</Button>
				)}
				{session !== null && (
					<>
						<Button
							size="sm"
							variant="ghost"
							disabled={chat.busy}
							onClick={() => void chat.exportMarkdown()}
						>
							{t("ui.chat.export")}
						</Button>
						{confirmDelete ? (
							<Button
								size="sm"
								variant="destructive"
								disabled={chat.busy}
								onClick={() => {
									setConfirmDelete(false);
									void chat.removeSession();
								}}
							>
								{t("ui.chat.delete_confirm")}
							</Button>
						) : (
							<Button
								size="sm"
								variant="ghost"
								disabled={chat.busy}
								onClick={() => setConfirmDelete(true)}
							>
								{t("ui.chat.delete")}
							</Button>
						)}
					</>
				)}
			</div>
			<p className="text-xs text-muted-foreground" data-testid="chat-state">
				{session === null
					? t("ui.chat.no_session")
					: `${t(`ui.chat.state_${state ?? "idle"}`)} · ${
							live
								? chat.live
									? t("ui.chat.live")
									: t("ui.chat.reconnecting")
								: t("ui.chat.not_running")
						}`}
				{session?.end_reason !== undefined && session.end_reason !== "" && (
					<>
						{" · "}
						{t(`ui.chat.ended_${session.end_reason}`)}
					</>
				)}
				{session !== null && (
					<>
						{" · "}
						<UntrustedText>{session.title}</UntrustedText>
					</>
				)}
			</p>
			<FailureNote failure={chat.failure} />
			<ol
				ref={log}
				role="log"
				aria-label={t("ui.chat.conversation")}
				aria-live="polite"
				aria-relevant="additions text"
				// biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable log must be reachable by keyboard
				tabIndex={0}
				onScroll={(event) => {
					const el = event.currentTarget;
					stuck.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
				}}
				className="flex min-h-40 flex-1 flex-col gap-2 overflow-y-auto rounded-lg border p-2 focus-visible:ring-3 focus-visible:ring-ring/50"
				data-testid="chat-log"
			>
				{events.filter(isVisible).map((event) => (
					<ChatEventView
						key={event.seq}
						event={event}
						open={waiting.has(event.permission?.request_id ?? "")}
						busy={chat.busy}
						onAnswer={(id, answer) => void chat.answer(id, answer)}
					/>
				))}
			</ol>
			<form
				onSubmit={(event) => void submit(event)}
				className="flex flex-col gap-2"
			>
				<Label htmlFor={`${ids}-message`} className="text-xs">
					{t("ui.chat.composer_label")}
				</Label>
				<Textarea
					id={`${ids}-message`}
					ref={composerRef}
					value={text}
					maxLength={20000}
					disabled={!canSend(state)}
					onChange={(event) => setText(event.target.value)}
					onKeyDown={onKey}
					className="min-h-16"
				/>
				<div className="flex items-center justify-between gap-2">
					<span className="text-xs text-muted-foreground">
						{t("ui.chat.composer_hint")}
					</span>
					<Button
						type="submit"
						size="sm"
						disabled={!canSend(state) || chat.busy || text.trim() === ""}
					>
						{t("ui.chat.send")}
					</Button>
				</div>
			</form>
		</div>
	);
}
