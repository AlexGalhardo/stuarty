import { Link } from "@inertiajs/react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import type { ApprovalRequest, InboxUndo } from "../../editor/api/types";
import { TargetStateBadge } from "../../pages/Publish/TargetStateBadge";
import { formatMoney } from "./money";
import type { ApprovalContext } from "./useApprovalContext";

// The words of the undo the service reported (D-140). A value the client does not know reads "Not verified",
// never "Can be undone".
const UNDO_KEY: Record<InboxUndo, string> = {
	can: "ui.approvals.undo_yes",
	cannot: "ui.approvals.undo_no",
	not_verified: "ui.approvals.undo_not_verified",
};

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
	const free = request.estimate.micros === 0;
	const { row, publication, target, post } = context;
	const notReported = <>{t("ui.approvals.not_reported")}</>;
	const text = (s: string | undefined): ReactNode =>
		s === undefined || s === "" ? (
			notReported
		) : (
			<UntrustedText>{s}</UntrustedText>
		);
	return (
		<li
			className="flex flex-col gap-3 p-4"
			data-approval-id={request.id}
			data-testid="approval-row"
		>
			<div className="min-w-0">
				<p className="text-xs text-muted-foreground">
					{t("ui.approvals.what")}
				</p>
				<p className="break-words font-medium" data-testid="approval-summary">
					{text(request.summary)}
				</p>
				{post !== undefined && (
					<div
						className="mt-2 rounded-lg border bg-muted/40 p-3 text-sm"
						data-testid="approval-post"
					>
						<p className="font-medium break-words">
							{text(publication?.title)}
						</p>
						<p
							className="mt-1 line-clamp-3 whitespace-pre-wrap break-words text-muted-foreground"
							data-testid="approval-excerpt"
						>
							{text(post.text.slice(0, 400))}
						</p>
						<p className="mt-1 text-xs text-muted-foreground">
							<span data-testid="approval-words">
								{t("ui.approvals.words", { count: post.words })}
							</span>
							{" · "}
							{t("ui.approvals.address")}{" "}
							<span data-testid="approval-address">{text(post.slug)}</span>
						</p>
					</div>
				)}
				<dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs text-muted-foreground">
					<dt>{t("ui.approvals.item")}</dt>
					<dd className="break-words" data-testid="approval-item">
						{text(publication?.title)}
					</dd>
					<dt>{t("ui.approvals.destination")}</dt>
					<dd className="break-words" data-testid="approval-destination">
						{text(destination ?? undefined)}
					</dd>
					<dt>{t("ui.approvals.cost")}</dt>
					<dd data-testid="approval-cost">
						{free
							? t("ui.approvals.no_cost")
							: formatMoney(request.estimate, locale)}
					</dd>
					<dt>{t("ui.approvals.undo")}</dt>
					<dd data-testid="approval-undo">
						{row === undefined
							? notReported
							: t(UNDO_KEY[row.undo] ?? UNDO_KEY.not_verified)}
					</dd>
					<dt>{t("ui.approvals.kind")}</dt>
					<dd className="break-words" data-testid="approval-tool">
						{text(request.kind)}
					</dd>
					<dt>{t("ui.approvals.skill")}</dt>
					<dd className="break-words" data-testid="approval-skill">
						{row === undefined ? notReported : text(row.skill)}
					</dd>
					<dt>{t("ui.approvals.path")}</dt>
					<dd data-testid="approval-path">
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
					</dd>
					<dt>{t("ui.approvals.actor")}</dt>
					<dd className="break-words" data-testid="approval-actor">
						<UntrustedText>{request.actor}</UntrustedText>
					</dd>
					<dt>{t("ui.approvals.asked")}</dt>
					<dd>{formatDateTime(request.created_at, locale)}</dd>
				</dl>
			</div>
			<div className="flex flex-wrap items-end gap-2">
				<div className="flex min-w-48 flex-1 flex-col gap-1">
					<Label htmlFor={`reason-${request.id}`} className="text-xs">
						{t("ui.approvals.reason")}
					</Label>
					<Input
						id={`reason-${request.id}`}
						autoComplete="off"
						maxLength={500}
						value={reason}
						onChange={(event) => setReason(event.target.value)}
					/>
				</div>
				<Button
					type="button"
					className="min-w-24"
					disabled={busy || !privileged}
					aria-describedby={privileged ? undefined : "approvals-person-only"}
					onClick={() => onDecide("approve", "")}
				>
					{t("ui.approvals.approve")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{request.summary}</UntrustedText>
					</span>
				</Button>
				<Button
					type="button"
					variant="outline"
					className="min-w-24"
					disabled={busy || !privileged}
					aria-describedby={privileged ? undefined : "approvals-person-only"}
					onClick={() => onDecide("reject", reason)}
				>
					{t("ui.approvals.reject")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{request.summary}</UntrustedText>
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
			<div className="flex items-center justify-between gap-4">
				<h2
					ref={heading}
					id="approvals-queue"
					tabIndex={-1}
					className="text-lg font-semibold"
				>
					{t("ui.approvals.queue")}
				</h2>
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
				<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
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
									summary: request.summary,
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
