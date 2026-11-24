import { Link } from "@inertiajs/react";
import {
	Bot,
	Brain,
	ListChecks,
	ShieldQuestion,
	Sparkles,
	SquareTerminal,
	User,
} from "lucide-react";
import type { ReactNode } from "react";

import { type Status, StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { ChatActivity, ChatEvent } from "../../editor/api/types";
import ActivityStrip from "./ActivityStrip";

const BODY = "whitespace-pre-wrap break-words text-sm text-text";

// The word of a tool status, with its tone: a word and an icon, never a colour alone.
const TOOL_STATUS: Record<
	NonNullable<NonNullable<ChatEvent["tool"]>["status"]>,
	Status
> = {
	pending: "waiting",
	in_progress: "agent",
	completed: "success",
	failed: "danger",
};

function Who({
	icon: Icon,
	children,
}: {
	icon: typeof Bot;
	children: ReactNode;
}): ReactNode {
	return (
		<p className="flex items-center gap-1.5 text-xs font-medium text-text-muted">
			<Icon aria-hidden="true" className="size-3.5 shrink-0" />
			{children}
		</p>
	);
}

// One line of the conversation. Everything the agent wrote is a string drawn as a text node: no
// HTML, no Markdown, no link made from it. A URL in it is text, in full (SDD-017 section 8).
export default function ChatEventView({
	event,
	open,
	busy,
	personOnly,
	agent,
	activity = null,
	onAnswer,
}: {
	event: ChatEvent;
	// the permission request is still waiting for a person
	open: boolean;
	busy: boolean;
	// the id of the note that says why, in a session that is not a person's: the server refuses its answer
	personOnly?: string;
	// the agent that runs the session (untrusted text) and what it is doing, for the card of a request that waits:
	// the same fields the scene bar shows, from the same state
	agent?: string | null;
	activity?: ChatActivity | null;
	onAnswer(requestId: string, answer: "allow" | "reject" | "cancel"): void;
}): ReactNode {
	const t = useT();
	const cannotAnswer = busy || personOnly !== undefined;

	switch (event.kind) {
		case "user_message":
			return (
				<li
					data-testid="chat-user"
					className="ms-8 rounded-lg bg-accent-soft px-3 py-2"
				>
					<Who icon={User}>{t("ui.chat.you")}</Who>
					<p className={BODY}>
						<UntrustedText>{event.text}</UntrustedText>
					</p>
				</li>
			);
		case "agent_message":
			return (
				<li
					data-testid="chat-agent"
					className="me-8 rounded-lg border border-border bg-surface px-3 py-2"
				>
					<Who icon={Bot}>{t("ui.chat.agent")}</Who>
					<p className={BODY}>
						<UntrustedText>{event.text}</UntrustedText>
					</p>
				</li>
			);
		case "agent_thought":
			return (
				<li data-testid="chat-thought" className="me-8 px-3 text-text-muted">
					<Who icon={Brain}>{t("ui.chat.thought")}</Who>
					<p className={`${BODY} italic text-text-muted`}>
						<UntrustedText>{event.text}</UntrustedText>
					</p>
				</li>
			);
		case "plan":
			return (
				<li data-testid="chat-plan" className="me-8 px-3 text-text-muted">
					<Who icon={ListChecks}>{t("ui.chat.plan")}</Who>
					<p className={`${BODY} text-text-muted`}>
						<UntrustedText>{event.text}</UntrustedText>
					</p>
				</li>
			);
		case "tool_call":
			return (
				<li
					data-testid="chat-tool"
					className="me-8 rounded-lg border border-dashed border-agent-border px-3 py-2"
				>
					<div className="flex flex-wrap items-center justify-between gap-2">
						<Who icon={SquareTerminal}>{t("ui.chat.tool")}</Who>
						<span data-testid="chat-tool-status">
							<StatusBadge
								status={TOOL_STATUS[event.tool?.status ?? "pending"]}
							>
								{t(`ui.chat.tool_status_${event.tool?.status ?? "pending"}`)}
							</StatusBadge>
						</span>
					</div>
					<p className={BODY}>
						<UntrustedText>{event.tool?.title}</UntrustedText>
					</p>
					{event.tool?.output !== undefined && event.tool.output !== "" && (
						<pre
							className="mt-1 max-h-40 overflow-auto rounded bg-surface-sunken p-2 text-xs whitespace-pre-wrap break-words text-text"
							data-testid="chat-tool-output"
						>
							<UntrustedText>{event.tool.output}</UntrustedText>
						</pre>
					)}
				</li>
			);
		case "permission_request":
			return (
				<li
					data-testid="chat-permission"
					className="me-4 flex flex-col gap-2 rounded-lg border-2 border-warning-text bg-warning-soft px-3 py-3"
				>
					<Who icon={ShieldQuestion}>{t("ui.chat.permission_title")}</Who>
					<p
						className={`${BODY} font-semibold`}
						data-testid="chat-permission-title"
					>
						<UntrustedText>{event.permission?.title}</UntrustedText>
					</p>
					{open && (
						<>
							{agent !== undefined && agent !== null && agent !== "" && (
								<p
									className="text-xs text-text-muted"
									data-testid="chat-permission-agent"
								>
									{`${t("ui.chat.agent_label")}: `}
									<UntrustedText>{agent}</UntrustedText>
								</p>
							)}
							<ActivityStrip
								activity={activity}
								testId="chat-permission-activity"
							/>
							<div className="flex flex-wrap items-center gap-2">
								<Button
									size="sm"
									disabled={cannotAnswer}
									aria-describedby={personOnly}
									onClick={() =>
										onAnswer(event.permission?.request_id ?? "", "allow")
									}
								>
									{t("ui.chat.allow_once")}
								</Button>
								<Button
									size="sm"
									variant="outline"
									disabled={cannotAnswer}
									aria-describedby={personOnly}
									onClick={() =>
										onAnswer(event.permission?.request_id ?? "", "reject")
									}
								>
									{t("ui.chat.reject")}
								</Button>
								<Button
									size="sm"
									variant="ghost"
									disabled={cannotAnswer}
									aria-describedby={personOnly}
									onClick={() =>
										onAnswer(event.permission?.request_id ?? "", "cancel")
									}
								>
									{t("ui.chat.cancel_request")}
								</Button>
							</div>
							<p className="text-xs text-text-muted">
								{t("ui.chat.same_as_bar")}
							</p>
						</>
					)}
					<p className="text-xs text-text-muted">
						{t("ui.chat.approvals_note")}{" "}
						<Link href="/approvals" className="underline">
							{t("ui.chat.approvals_link")}
						</Link>
					</p>
				</li>
			);
		case "permission_answer":
			return (
				<li data-testid="chat-answer" className="px-3 text-xs text-text-muted">
					{t(`ui.chat.answer_${event.permission?.answer ?? "cancel"}`)}{" "}
					<UntrustedText>{event.permission?.title}</UntrustedText>
				</li>
			);
		case "skill_loaded":
			return (
				<li
					data-testid="chat-skill"
					className="flex flex-wrap items-center gap-1.5 px-3 text-sm text-agent-text"
				>
					<Sparkles aria-hidden="true" className="size-3.5 shrink-0" />
					{t("ui.chat.skill_loaded")}:{" "}
					<UntrustedText>{event.skill?.name}</UntrustedText>
				</li>
			);
		case "notice":
			return (
				<li data-testid="chat-notice" className="px-3 text-xs text-text-muted">
					<UntrustedText>{event.text}</UntrustedText>
				</li>
			);
		default:
			return null;
	}
}
