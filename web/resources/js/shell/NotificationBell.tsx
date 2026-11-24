import { Link } from "@inertiajs/react";
import {
	Ban,
	Bell,
	CircleCheck,
	Hourglass,
	type LucideIcon,
} from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Sheet,
	SheetContent,
	SheetDescription,
	SheetHeader,
	SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import {
	type Notification,
	type NotificationState,
	notificationsOf,
	waitingCount,
} from "./notifications";
import { useShell } from "./ShellContext";
import { projectNameOf } from "./sceneState";

const STATE: Record<NotificationState, { icon: LucideIcon; tone: string }> = {
	waiting: { icon: Hourglass, tone: "text-warning-text" },
	allowed: { icon: CircleCheck, tone: "text-success-text" },
	denied: { icon: Ban, tone: "text-danger-text" },
};

// One request in the center. The agent and the title are text an agent or a job kind wrote. A permission is
// answered here with the same two choices as on the scene bar (never "always"); an approval is decided on
// its own page, which the row links to.
function Row({
	item,
	project,
	busy,
	disabledBy,
	onAnswer,
	onOpenChat,
	onNavigate,
}: {
	item: Notification;
	// the project of the chat session (untrusted text), named on a permission row
	project: string | null;
	busy: boolean;
	disabledBy: string | undefined;
	onAnswer(requestId: string, answer: "allow" | "reject"): void;
	onOpenChat(): void;
	onNavigate(): void;
}): ReactNode {
	const t = useT();
	const { icon: Icon, tone } = STATE[item.state];
	const waiting = item.state === "waiting";
	return (
		<li
			data-testid="notification"
			data-kind={item.kind}
			data-state={item.state}
			className={cn(
				"flex flex-col gap-2 rounded-lg border p-3",
				waiting ? "border-warning bg-warning-soft" : "bg-surface",
			)}
		>
			<p className={cn("flex items-center gap-1.5 text-xs font-medium", tone)}>
				<Icon aria-hidden="true" className="size-4 shrink-0" />
				{t(`ui.shell.notify.state.${item.state}`)}
				<span className="text-muted-foreground">
					· {t(`ui.shell.notify.kind.${item.kind}`)}
				</span>
			</p>
			<div className="min-w-0">
				<p className="truncate text-xs text-muted-foreground">
					<UntrustedText>{item.agent}</UntrustedText>
					{item.kind === "permission" && project !== null && (
						<span data-testid="notification-project">
							{" · "}
							<span className="sr-only">{t("ui.shell.scene.project")} </span>
							<UntrustedText>{project}</UntrustedText>
						</span>
					)}
				</p>
				<p className="text-sm font-medium break-words">
					<UntrustedText>{item.title}</UntrustedText>
				</p>
			</div>
			{waiting && item.kind === "permission" && (
				<div className="flex flex-wrap items-center gap-2">
					<Button
						type="button"
						size="sm"
						variant="outline"
						disabled={busy || disabledBy !== undefined}
						aria-describedby={disabledBy}
						onClick={() => onAnswer(item.id, "reject")}
						data-testid="notification-deny"
					>
						{t("ui.shell.scene.deny")}
					</Button>
					<Button
						type="button"
						size="sm"
						disabled={busy || disabledBy !== undefined}
						aria-describedby={disabledBy}
						onClick={() => onAnswer(item.id, "allow")}
						data-testid="notification-allow"
					>
						{t("ui.shell.scene.allow")}
					</Button>
					<Button
						type="button"
						size="sm"
						variant="ghost"
						onClick={onOpenChat}
						data-testid="notification-open-chat"
					>
						{t("ui.shell.notify.open_chat")}
					</Button>
				</div>
			)}
			{waiting && item.kind === "approval" && (
				<Link
					href="/approvals"
					onClick={onNavigate}
					data-testid="notification-review"
					className="w-fit rounded text-sm font-medium text-accent-text underline underline-offset-2"
				>
					{t("ui.shell.notify.review")}
				</Link>
			)}
		</li>
	);
}

// What the center holds under its title: the failure of the queue, the rows or the empty state, and the
// reason a session that is not a person's cannot answer.
export function CenterBody({
	items,
	project = null,
	failure,
	busy,
	personOnlyId,
	privileged,
	onAnswer,
	onOpenChat,
	onNavigate,
}: {
	items: readonly Notification[];
	project?: string | null;
	failure: Failure | null;
	busy: boolean;
	personOnlyId: string;
	privileged: boolean;
	onAnswer(requestId: string, answer: "allow" | "reject"): void;
	onOpenChat(): void;
	onNavigate(): void;
}): ReactNode {
	const t = useT();
	return (
		<>
			<FailureNote failure={failure} />
			{items.length === 0 ? (
				<p
					data-testid="notification-empty"
					className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground"
				>
					{t("ui.shell.notify.empty")}
				</p>
			) : (
				<ul className="flex flex-col gap-2">
					{items.map((item) => (
						<Row
							key={`${item.kind}-${item.id}`}
							item={item}
							project={project}
							busy={busy}
							disabledBy={privileged ? undefined : personOnlyId}
							onAnswer={onAnswer}
							onOpenChat={onOpenChat}
							onNavigate={onNavigate}
						/>
					))}
				</ul>
			)}
			{waitingCount(items) > 0 && <PersonOnlyNote id={personOnlyId} />}
		</>
	);
}

// The bell of the top bar and the center it opens (SDD-012 section 23.4). The count is what waits for a
// person: the open permission requests of the chat session the shell reads plus the pending approvals.
export default function NotificationBell(): ReactNode {
	const t = useT();
	const { privileged } = useSession();
	const ids = useId();
	const { chat, approvals, openChat } = useShell();
	const [open, setOpen] = useState(false);
	const items = notificationsOf(chat.session, chat.events, approvals.requests);
	const waiting = waitingCount(items);

	return (
		<>
			<Button
				type="button"
				variant="outline"
				size="icon"
				aria-label={
					waiting > 0
						? t("ui.shell.notify.bell_waiting", { count: waiting })
						: t("ui.shell.notify.bell")
				}
				aria-haspopup="dialog"
				aria-expanded={open}
				onClick={() => setOpen(true)}
				className="relative"
				data-testid="bell"
			>
				<Bell aria-hidden="true" />
				{waiting > 0 && (
					<span
						aria-hidden="true"
						data-testid="bell-count"
						className="absolute -end-1.5 -top-1.5 min-w-5 rounded-full bg-warning-soft px-1 text-center text-xs font-medium text-warning-text ring-1 ring-warning tabular-nums"
					>
						{waiting}
					</span>
				)}
			</Button>
			<Sheet open={open} onOpenChange={setOpen}>
				<SheetContent
					side="right"
					data-testid="notification-center"
					className="w-full gap-3 overflow-y-auto p-4 sm:max-w-md"
				>
					<SheetHeader className="p-0 pe-10">
						<SheetTitle className="flex flex-wrap items-center gap-2 text-base">
							{t("ui.shell.notify.title")}
							<span
								data-testid="center-count"
								className={cn(
									"rounded-full px-2 py-0.5 text-xs font-medium",
									waiting > 0
										? "bg-warning-soft text-warning-text"
										: "bg-surface-hover text-text-muted",
								)}
							>
								{t("ui.shell.waiting_count", { count: waiting })}
							</span>
						</SheetTitle>
						<SheetDescription>{t("ui.shell.notify.scope")}</SheetDescription>
					</SheetHeader>
					<CenterBody
						items={items}
						project={projectNameOf(chat.session, chat.projects)}
						failure={approvals.failure}
						busy={chat.busy}
						personOnlyId={`${ids}-person-only`}
						privileged={privileged}
						onAnswer={(requestId, answer) =>
							void chat.answer(requestId, answer)
						}
						onOpenChat={() => {
							setOpen(false);
							openChat();
						}}
						onNavigate={() => setOpen(false)}
					/>
				</SheetContent>
			</Sheet>
		</>
	);
}
