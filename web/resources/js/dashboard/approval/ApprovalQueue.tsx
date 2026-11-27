import { Link } from "@inertiajs/react";
import { Check, Lock, X } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { FieldLabel } from "@/components/FieldHelp";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import type { ApprovalRequest, InboxUndo } from "../../editor/api/types";
import { TargetStateBadge } from "../../pages/Publish/TargetStateBadge";
import { headline } from "./humanize";
import { kindIcon } from "./kindIcon";
import { formatMoney } from "./money";
import type { ApprovalContext } from "./useApprovalContext";

// The words of the undo the service reported (D-140). A value the client does not know reads "Not verified",
// never "Can be undone".
const UNDO_KEY: Record<InboxUndo, string> = {
	can: "ui.approvals.undo_yes",
	cannot: "ui.approvals.undo_no",
	not_verified: "ui.approvals.undo_not_verified",
};

// One fact of a request: the label above its value, as the final design sets them.
function Fact({
	label,
	testId,
	mono = false,
	children,
}: {
	label: string;
	testId?: string;
	mono?: boolean;
	children: ReactNode;
}): ReactNode {
	return (
		<div className="min-w-0">
			<dt className="text-xs text-text-subtle">{label}</dt>
			<dd
				className={cn(
					"break-words font-medium text-text",
					mono && "font-mono text-[13px]",
				)}
				data-testid={testId}
			>
				{children}
			</dd>
		</div>
	);
}

const NO_CONTEXT: ApprovalContext = {};

// One request of the queue (SDD-015, RF-362): what happens, the item, where it goes, what it costs, whether it
// can be undone, who prepared it with which skill and tool, and the path to the publication, then the two
// answers of a person at the same size. Every member is the service's: a member that did not arrive reads "Not
// reported", never a guess. The item decides nothing: it sends the answer and the service says whether it stands.
export function ApprovalItem({
	request,
	destination,
	context = NO_CONTEXT,
	busy,
	onDecide,
}: {
	request: ApprovalRequest;
	// the label of the connection, or null when the action has none or it is gone
	destination: string | null;
	context?: ApprovalContext;
	busy: boolean;
	onDecide(decision: "approve" | "reject", reason: string): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const { privileged } = useSession();
	const [reason, setReason] = useState("");
	// The reason is asked only when a person starts to reject: the first press opens the field, the second sends.
	const [rejecting, setRejecting] = useState(false);
	const free = request.estimate.micros === 0;
	const { row, publication, target, post } = context;
	const notReported = <>{t("ui.approvals.not_reported")}</>;
	const text = (s: string | undefined): ReactNode =>
		s === undefined || s === "" ? (
			notReported
		) : (
			<UntrustedText>{s}</UntrustedText>
		);
	const Icon = kindIcon(request.kind);
	// the title is worded here from the kind, the item and the place: the service's English summary is not shown
	const title = headline(t, request.kind, publication?.title, destination);
	return (
		<li
			className="flex flex-col gap-4 rounded-lg border border-warning bg-surface p-4"
			data-approval-id={request.id}
			data-testid="approval-row"
		>
			<div className="flex items-start gap-3">
				<span
					aria-hidden="true"
					className="flex size-10 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent-text"
				>
					<Icon className="size-5" />
				</span>
				<div className="min-w-0 flex-1">
					<p className="sr-only">{t("ui.approvals.what")}</p>
					<p
						className="break-words text-base font-semibold text-text"
						data-testid="approval-title"
					>
						<UntrustedText>{title}</UntrustedText>
					</p>
					<details className="mt-1 text-sm text-text-subtle">
						<summary className="w-fit cursor-pointer">
							{t("ui.approvals.original")}
						</summary>
						<p className="mt-1 break-words" data-testid="approval-summary">
							{text(request.summary)}
						</p>
					</details>
				</div>
				<StatusBadge status="waiting">{t("ui.approvals.queue")}</StatusBadge>
			</div>
			<div
				className={
					post === undefined
						? undefined
						: "grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]"
				}
			>
				<dl
					className={cn(
						"grid grid-cols-2 gap-x-6 gap-y-3 text-sm",
						post === undefined && "md:grid-cols-3",
					)}
				>
					<Fact label={t("ui.approvals.actor")} testId="approval-actor">
						<UntrustedText>{request.actor}</UntrustedText>
					</Fact>
					<Fact label={t("ui.approvals.skill")} testId="approval-skill">
						{row === undefined ? notReported : text(row.skill)}
					</Fact>
					<Fact label={t("ui.approvals.kind")} testId="approval-tool" mono>
						{text(request.kind)}
					</Fact>
					<Fact label={t("ui.approvals.item")} testId="approval-item">
						{text(publication?.title)}
					</Fact>
					<Fact
						label={t("ui.approvals.destination")}
						testId="approval-destination"
					>
						{text(destination ?? undefined)}
					</Fact>
					<Fact label={t("ui.approvals.cost")} testId="approval-cost">
						{free
							? t("ui.approvals.no_cost")
							: formatMoney(request.estimate, locale)}
					</Fact>
					<Fact label={t("ui.approvals.undo")} testId="approval-undo">
						{row === undefined
							? notReported
							: t(UNDO_KEY[row.undo] ?? UNDO_KEY.not_verified)}
					</Fact>
					<Fact label={t("ui.approvals.path")} testId="approval-path">
						{target === undefined || publication === undefined ? (
							notReported
						) : (
							<span className="flex flex-wrap items-center gap-2">
								<TargetStateBadge state={target.state} />
								<Link href="/publish" className="underline">
									{t("ui.approvals.open_publish")}
								</Link>
							</span>
						)}
					</Fact>
					<Fact label={t("ui.approvals.asked")}>
						{formatDateTime(request.created_at, locale)}
					</Fact>
				</dl>
				{post !== undefined && (
					<aside
						aria-label={t("ui.approvals.post_title")}
						className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-surface-sunken p-3 text-sm"
						data-testid="approval-post"
					>
						<h3 className="text-xs font-semibold uppercase tracking-wide text-text-subtle">
							{t("ui.approvals.post_title")}
						</h3>
						<p className="font-medium break-words">
							{text(publication?.title)}
						</p>
						<p
							className="max-h-72 overflow-y-auto whitespace-pre-wrap break-words text-text-muted"
							data-testid="approval-excerpt"
						>
							{text(post.text)}
						</p>
						<p className="text-xs text-text-subtle">
							<span data-testid="approval-words">
								{t("ui.approvals.words", { count: post.words })}
							</span>
							{" · "}
							{t("ui.approvals.address")}{" "}
							<span data-testid="approval-address">{text(post.slug)}</span>
						</p>
					</aside>
				)}
			</div>
			<div className="flex flex-wrap items-end gap-3">
				<p className="flex min-w-48 flex-1 items-center gap-2 text-sm text-text-subtle">
					<Lock aria-hidden="true" className="size-4 shrink-0" />
					{t("ui.approvals.person_decides")}
				</p>
				{rejecting && (
					<div className="flex min-w-48 flex-col gap-1">
						<FieldLabel
							htmlFor={`reason-${request.id}`}
							label={t("ui.approvals.reason")}
							labelClassName="text-xs"
							help={t("ui.help.g3.approval_reason_help")}
						/>
						<Input
							autoFocus
							id={`reason-${request.id}`}
							placeholder={t("ui.help.g3.approval_reason_ph")}
							autoComplete="off"
							maxLength={500}
							value={reason}
							onChange={(event) => setReason(event.target.value)}
						/>
					</div>
				)}
				<Button
					type="button"
					variant="outline"
					size="lg"
					className="min-w-24"
					disabled={busy || !privileged}
					aria-describedby={privileged ? undefined : "approvals-person-only"}
					onClick={() => {
						if (rejecting) onDecide("reject", reason);
						else setRejecting(true);
					}}
				>
					<X aria-hidden="true" />
					{t("ui.approvals.reject")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{title}</UntrustedText>
					</span>
				</Button>
				<Button
					type="button"
					size="lg"
					className="min-w-24"
					disabled={busy || !privileged}
					aria-describedby={privileged ? undefined : "approvals-person-only"}
					onClick={() => onDecide("approve", "")}
				>
					<Check aria-hidden="true" />
					{t("ui.approvals.approve")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{title}</UntrustedText>
					</span>
				</Button>
			</div>
		</li>
	);
}

// The queue: every request that waits for a person, newest first, updated by the stream without a
// reload. With nothing waiting it says so.
export default function ApprovalQueue({
	requests,
	destinations,
	context = {},
	loading = false,
	failed = false,
	busyId,
	live,
	onDecide,
}: {
	requests: readonly ApprovalRequest[];
	destinations: Readonly<Record<string, string>>;
	// what the service says about each request beyond the request itself, by id
	context?: Readonly<Record<string, ApprovalContext>>;
	// the first answer has not come: the queue is not empty, it is not known yet
	loading?: boolean;
	// the read failed: the page shows the failure and the queue says nothing about being empty
	failed?: boolean;
	busyId: string | null;
	live: boolean;
	onDecide(id: string, decision: "approve" | "reject", reason: string): void;
}): ReactNode {
	const t = useT();
	const heading = useRef<HTMLHeadingElement>(null);
	const section = useRef<HTMLElement>(null);
	// The request a person just decided: its row leaves the list, and the focus that was on its button
	// would fall to the page, so it moves to the next request, or to the heading when none is left
	// (WCAG 2.4.3), and a polite region says what was decided (WCAG 4.1.3).
	const decided = useRef<{
		id: string;
		index: number;
		decision: "approve" | "reject";
		summary: string;
	} | null>(null);
	const [result, setResult] = useState("");
	const count = requests.length;
	// biome-ignore lint/correctness/useExhaustiveDependencies: runs when the number of requests changes
	useEffect(() => {
		const last = decided.current;
		if (last === null || requests.some((request) => request.id === last.id)) {
			return;
		}
		decided.current = null;
		setResult(
			t(`ui.approvals.result_${last.decision}`, { summary: last.summary }),
		);
		const next = requests[Math.min(last.index, requests.length - 1)];
		const button =
			next === undefined
				? null
				: section.current?.querySelector<HTMLElement>(
						`[data-approval-id="${next.id}"] button`,
					);
		(button ?? heading.current)?.focus();
	}, [count]);
	return (
		<section ref={section} aria-labelledby="approvals-queue">
			<p role="status" className="sr-only" data-testid="approval-result">
				{result}
			</p>
			<div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
				<div className="flex items-baseline gap-3">
					<h2
						ref={heading}
						id="approvals-queue"
						tabIndex={-1}
						className="text-xl font-semibold"
					>
						{t("ui.approvals.queue")}
					</h2>
					{requests.length > 0 && (
						<span
							className="text-sm text-text-subtle"
							data-testid="approval-count"
						>
							{t("ui.approvals.count", { count: requests.length })}
						</span>
					)}
				</div>
				<p
					className="text-xs text-muted-foreground"
					role="status"
					data-testid="approval-live"
					data-live={live ? "true" : "false"}
				>
					{live ? t("ui.approvals.live") : t("ui.approvals.offline")}
				</p>
			</div>
			{requests.length > 0 && (
				<div className="mt-2">
					<PersonOnlyNote id="approvals-person-only" />
				</div>
			)}
			{requests.length === 0 ? (
				// The queue not read is not an empty queue: "nothing is waiting" is said only when the service said so.
				loading && failed ? null : (
					<p
						className="mt-4 text-sm text-muted-foreground"
						data-testid={loading ? "approval-loading" : "approval-empty"}
					>
						{loading ? t("ui.approvals.loading") : t("ui.approvals.empty")}
					</p>
				)
			) : (
				<ul className="mt-4 flex flex-col gap-4">
					{requests.map((request) => (
						<ApprovalItem
							key={request.id}
							request={request}
							context={context[request.id]}
							destination={
								request.connection_id === null
									? null
									: (destinations[request.connection_id] ?? null)
							}
							busy={busyId === request.id}
							onDecide={(decision, reason) => {
								decided.current = {
									id: request.id,
									index: requests.indexOf(request),
									decision,
									summary: headline(
										t,
										request.kind,
										context[request.id]?.publication?.title,
										request.connection_id === null
											? null
											: (destinations[request.connection_id] ?? null),
									),
								};
								onDecide(request.id, decision, reason);
							}}
						/>
					))}
				</ul>
			)}
		</section>
	);
}
