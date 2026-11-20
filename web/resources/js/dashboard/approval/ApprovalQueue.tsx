import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import type { ApprovalRequest } from "../../editor/api/types";
import { formatMoney } from "./money";

// One request of the queue: what will go out (the summary an agent wrote, as text), where, what it
// costs, who prepared it and when, and the two answers of a person (SDD-015, RF-362). The row decides
// nothing: it sends the answer and the service says whether it stands.
export function ApprovalRow({
	request,
	destination,
	busy,
	onDecide,
}: {
	request: ApprovalRequest;
	// the label of the connection, or null when the action has none or it is gone
	destination: string | null;
	busy: boolean;
	onDecide(decision: "approve" | "reject", reason: string): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const { privileged } = useSession();
	const [reason, setReason] = useState("");
	const free = request.estimate.micros === 0;
	return (
		<li
			className="flex flex-col gap-3 p-4"
			data-approval-id={request.id}
			data-testid="approval-row"
		>
			<div className="min-w-0">
				<p className="font-medium" data-testid="approval-summary">
					<UntrustedText>{request.summary}</UntrustedText>
				</p>
				<dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs text-muted-foreground">
					<dt>{t("ui.approvals.kind")}</dt>
					<dd>
						<UntrustedText>{request.kind}</UntrustedText>
					</dd>
					<dt>{t("ui.approvals.destination")}</dt>
					<dd data-testid="approval-destination">
						{destination === null ? (
							t("ui.approvals.no_destination")
						) : (
							<UntrustedText>{destination}</UntrustedText>
						)}
					</dd>
					<dt>{t("ui.approvals.cost")}</dt>
					<dd data-testid="approval-cost">
						{free
							? t("ui.approvals.no_cost")
							: formatMoney(request.estimate, locale)}
					</dd>
					<dt>{t("ui.approvals.actor")}</dt>
					<dd data-testid="approval-actor">
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
	busyId,
	live,
	onDecide,
}: {
	requests: readonly ApprovalRequest[];
	destinations: Readonly<Record<string, string>>;
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
				<p
					className="mt-4 text-sm text-muted-foreground"
					data-testid="approval-empty"
				>
					{t("ui.approvals.empty")}
				</p>
			) : (
				<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
					{requests.map((request) => (
						<ApprovalRow
							key={request.id}
							request={request}
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
