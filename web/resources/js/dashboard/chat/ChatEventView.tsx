import { Link } from "@inertiajs/react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { ChatEvent } from "../../editor/api/types";

// One line of the conversation. Everything the agent wrote is a string drawn as a text node: no
// HTML, no Markdown, no link made from it. A URL in it is text, in full (SDD-017 section 8).
export default function ChatEventView({
	event,
	open,
	busy,
	onAnswer,
}: {
	event: ChatEvent;
	// the permission request is still waiting for a person
	open: boolean;
	busy: boolean;
	onAnswer(requestId: string, answer: "allow" | "reject" | "cancel"): void;
}): ReactNode {
	const t = useT();
	const body = "whitespace-pre-wrap break-words text-sm";

	switch (event.kind) {
		case "user_message":
			return (
				<li
					data-testid="chat-user"
					className="ms-6 rounded-lg bg-primary/10 px-3 py-2"
				>
					<p className="text-xs font-medium text-muted-foreground">
						{t("ui.chat.you")}
					</p>
					<p className={body}>
						<UntrustedText>{event.text}</UntrustedText>
					</p>
				</li>
			);
		case "agent_message":
			return (
				<li
					data-testid="chat-agent"
					className="me-6 rounded-lg border bg-card px-3 py-2"
				>
					<p className="text-xs font-medium text-muted-foreground">
						{t("ui.chat.agent")}
					</p>
					<p className={body}>
						<UntrustedText>{event.text}</UntrustedText>
					</p>
				</li>
			);
		case "agent_thought":
			return (
				<li
					data-testid="chat-thought"
					className="me-6 px-3 text-muted-foreground"
				>
					<p className="text-xs font-medium">{t("ui.chat.thought")}</p>
					<p className={`${body} italic`}>
						<UntrustedText>{event.text}</UntrustedText>
					</p>
				</li>
			);
		case "plan":
			return (
				<li data-testid="chat-plan" className="me-6 px-3 text-muted-foreground">
					<p className="text-xs font-medium">{t("ui.chat.plan")}</p>
					<p className={body}>
						<UntrustedText>{event.text}</UntrustedText>
					</p>
				</li>
			);
		case "tool_call":
			return (
				<li
					data-testid="chat-tool"
					className="me-6 rounded-lg border border-dashed px-3 py-2"
				>
					<p className="text-xs font-medium text-muted-foreground">
						{t("ui.chat.tool")}{" "}
						<span data-testid="chat-tool-status">
							{t(`ui.chat.tool_status_${event.tool?.status ?? "pending"}`)}
						</span>
					</p>
					<p className={body}>
						<UntrustedText>{event.tool?.title}</UntrustedText>
					</p>
					{event.tool?.output !== undefined && event.tool.output !== "" && (
						<pre
							className="mt-1 max-h-40 overflow-auto rounded bg-muted p-2 text-xs whitespace-pre-wrap break-words"
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
					className="me-6 rounded-lg border-2 border-ring px-3 py-2"
				>
					<p className="text-xs font-medium">{t("ui.chat.permission_title")}</p>
					<p
						className={`${body} font-medium`}
						data-testid="chat-permission-title"
					>
						<UntrustedText>{event.permission?.title}</UntrustedText>
					</p>
					{open ? (
						<div className="mt-2 flex flex-wrap items-center gap-2">
							<Button
								size="sm"
								disabled={busy}
								onClick={() =>
									onAnswer(event.permission?.request_id ?? "", "allow")
								}
							>
								{t("ui.chat.allow_once")}
							</Button>
							<Button
								size="sm"
								variant="outline"
								disabled={busy}
								onClick={() =>
									onAnswer(event.permission?.request_id ?? "", "reject")
								}
							>
								{t("ui.chat.reject")}
							</Button>
							<Button
								size="sm"
								variant="ghost"
								disabled={busy}
								onClick={() =>
									onAnswer(event.permission?.request_id ?? "", "cancel")
								}
							>
								{t("ui.chat.cancel_request")}
							</Button>
						</div>
					) : null}
					<p className="mt-2 text-xs text-muted-foreground">
						{t("ui.chat.approvals_note")}{" "}
						<Link href="/approvals" className="underline">
							{t("ui.chat.approvals_link")}
						</Link>
					</p>
				</li>
			);
		case "permission_answer":
			return (
				<li
					data-testid="chat-answer"
					className="px-3 text-xs text-muted-foreground"
				>
					{t(`ui.chat.answer_${event.permission?.answer ?? "cancel"}`)}{" "}
					<UntrustedText>{event.permission?.title}</UntrustedText>
				</li>
			);
		case "notice":
			return (
				<li
					data-testid="chat-notice"
					className="px-3 text-xs text-muted-foreground"
				>
					<UntrustedText>{event.text}</UntrustedText>
				</li>
			);
		default:
			return null;
	}
}
