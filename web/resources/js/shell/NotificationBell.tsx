import { Link } from "@inertiajs/react";
import {
	Ban,
	Bell,
	CircleCheck,
	CircleSlash,
	Clock,
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
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import { formatMoney } from "../dashboard/approval/money";
import type { InboxUndo } from "../editor/api/types";
import { type Notification, waitingCount } from "./notifications";
import { useShell } from "./ShellContext";

// How a row says its state: a word and an icon, never colour alone. A permission of the chat is "allowed" or
// "denied"; an approval is "approved" or "rejected"; the others are the same for both.
type StateWord =
	| "waiting"
	| "allowed"
	| "denied"
	| "approved"
	| "rejected"
	| "expired"
	| "cancelled";

const STATE: Record<StateWord, { icon: LucideIcon; tone: string }> = {
	waiting: { icon: Hourglass, tone: "text-warning-text" },
	allowed: { icon: CircleCheck, tone: "text-success-text" },
	denied: { icon: Ban, tone: "text-danger-text" },
	approved: { icon: CircleCheck, tone: "text-success-text" },
	rejected: { icon: Ban, tone: "text-danger-text" },
	expired: { icon: Clock, tone: "text-muted-foreground" },
	cancelled: { icon: CircleSlash, tone: "text-muted-foreground" },
};

// The words of the undo the service reported (D-140). A value the client does not know reads "Not verified",
// never "Can be undone".
const UNDO_KEY: Record<InboxUndo, string> = {
	can: "ui.shell.notify.undo_yes",
	cannot: "ui.shell.notify.undo_no",
	not_verified: "ui.shell.notify.undo_not_verified",
};

function stateWord(item: Notification): StateWord {
	if (item.source === "chat") {
		if (item.state === "approved") return "allowed";
		if (item.state === "rejected") return "denied";
	}
	return item.state;
}

// One line of the facts of a request: the label, and the value or "Not reported". Nothing is guessed.
function Fact({
	label,
	testId,
	children,
}: {
	label: string;
	testId: string;
	children: ReactNode;
}): ReactNode {
	return (
		<>
			<dt className="text-muted-foreground">{label}</dt>
			<dd data-testid={testId} className="min-w-0 break-words">
				{children}
			</dd>
		</>
	);
}

// One request in the center. Every text but the state, the cost and the dates is something an agent or a job
// kind wrote and is shown as text. A permission is answered here with the same two choices as on the scene bar
// (never "always"); an approval is decided on its own page, which the row links to. The row decides nothing
// itself: it calls the endpoint of its source.
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
	// the name of the project of the request (untrusted text), when the project list holds it
	project: string | null;
	busy: boolean;
	disabledBy: string | undefined;
	onAnswer(row: Notification, answer: "allow" | "reject"): void;
	onOpenChat(): void;
	onNavigate(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const word = stateWord(item);
	const { icon: Icon, tone } = STATE[word];
	const waiting = item.state === "waiting";
	const chat = item.source === "chat";
	const notReported = <>{t("ui.shell.scene.not_reported")}</>;
	const text = (s: string): ReactNode =>
		s === "" ? notReported : <UntrustedText>{s}</UntrustedText>;
	// what will happen, in plain words when the service knows the tool; the title the agent wrote stays beside
	// it, because the recognition reads only that title
	const what =
		item.action_key !== "" ? (
			<>
				{t(item.action_key)}{" "}
				<span className="font-normal text-muted-foreground">
					(<UntrustedText>{item.tool}</UntrustedText>)
				</span>
			</>
		) : (
			text(item.tool)
		);
	return (
		<li
			data-testid="notification"
			data-kind={chat ? "permission" : "approval"}
			data-source={item.source}
			data-state={word}
			className={cn(
				"flex flex-col gap-2 rounded-lg border p-3",
				waiting ? "border-warning bg-warning-soft" : "bg-surface",
			)}
		>
			<p className={cn("flex items-center gap-1.5 text-xs font-medium", tone)}>
				<Icon aria-hidden="true" className="size-4 shrink-0" />
				{t(`ui.shell.notify.state.${word}`)}
				<span className="text-muted-foreground">
					· {t(`ui.shell.notify.kind.${chat ? "permission" : "approval"}`)}
				</span>
			</p>
			<div className="min-w-0">
				<p className="relative truncate text-xs text-muted-foreground">
					<UntrustedText>{item.agent}</UntrustedText>
					{chat && project !== null && (
						<span data-testid="notification-project">
							{" · "}
							<span className="sr-only">{t("ui.shell.scene.project")} </span>
							<UntrustedText>{project}</UntrustedText>
						</span>
					)}
				</p>
				<p
					data-testid="notification-what"
					className="text-sm font-medium break-words"
				>
					{what}
				</p>
			</div>
			<dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
				<Fact label={t("ui.shell.notify.item")} testId="notification-item">
					{text(chat ? item.item : item.summary)}
				</Fact>
				<Fact
					label={t("ui.shell.notify.where")}
					testId="notification-destination"
				>
					{text(item.destination)}
				</Fact>
				<Fact label={t("ui.shell.notify.cost")} testId="notification-cost">
					{item.cost === null
						? t("ui.shell.notify.no_cost")
						: formatMoney(item.cost, locale)}
				</Fact>
				<Fact label={t("ui.shell.notify.undo")} testId="notification-undo">
					{t(UNDO_KEY[item.undo] ?? UNDO_KEY.not_verified)}
				</Fact>
				{chat && (
					<Fact label={t("ui.shell.scene.skill")} testId="notification-skill">
						{item.skill === "" ? (
							t("ui.shell.scene.skill_state.none")
						) : (
							<>
								<UntrustedText>{item.skill}</UntrustedText>{" "}
								<span className="text-muted-foreground">
									({t("ui.shell.scene.skill_state.loaded")})
								</span>
							</>
						)}
					</Fact>
				)}
				<Fact label={t("ui.shell.notify.asked")} testId="notification-asked">
					{formatDateTime(item.created_at, locale)}
				</Fact>
			</dl>
			{chat && item.action_key !== "" && (
				<p className="text-xs text-muted-foreground">
					{t("ui.shell.notify.agent_reported")}
				</p>
			)}
			{waiting && chat && (
				<div className="flex flex-wrap items-center gap-2">
					<Button
						type="button"
						size="sm"
						variant="outline"
						disabled={busy || disabledBy !== undefined}
						aria-describedby={disabledBy}
						onClick={() => onAnswer(item, "reject")}
						data-testid="notification-deny"
					>
						{t("ui.shell.scene.deny")}
					</Button>
					<Button
						type="button"
						size="sm"
						disabled={busy || disabledBy !== undefined}
						aria-describedby={disabledBy}
						onClick={() => onAnswer(item, "allow")}
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
			{waiting && !chat && (
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

// What the center holds under its title: the failure of the read, the rows or the empty state, and the
// reason a session that is not a person's cannot answer.
export function CenterBody({
	items,
	projectName = () => null,
	failure,
	busyId,
	personOnlyId,
	privileged,
	onAnswer,
	onOpenChat,
	onNavigate,
}: {
	items: readonly Notification[];
	// the name of a project by its id (untrusted text); null when the list does not hold it
	projectName?(id: string): string | null;
	failure: Failure | null;
	busyId: string | null;
	personOnlyId: string;
	privileged: boolean;
	onAnswer(row: Notification, answer: "allow" | "reject"): void;
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
							key={item.id}
							item={item}
							project={projectName(item.project_id)}
							busy={busyId !== null}
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
// person: the rows of the one list the service builds from the chat permissions and the approvals.
export default function NotificationBell(): ReactNode {
	const t = useT();
	const { privileged } = useSession();
	const ids = useId();
	const { chat, inbox, openChat } = useShell();
	const [open, setOpen] = useState(false);
	const items = inbox.rows ?? [];
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
						projectName={(id) =>
							chat.projects.find((p) => p.id === id)?.name ?? null
						}
						failure={inbox.failure}
						busyId={inbox.busyId}
						personOnlyId={`${ids}-person-only`}
						privileged={privileged}
						onAnswer={(row, answer) => void inbox.answer(row, answer)}
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
