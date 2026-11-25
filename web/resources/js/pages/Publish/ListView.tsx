import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { formatDateTime, formatInZone } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { linkOf } from "./publicationState";
import { TargetStateBadge } from "./TargetStateBadge";
import type { Publication, Target } from "./types";

// Whether TargetResult renders a concrete outcome (explicit URL/draft/error or a known status note).
export function hasTargetResult(target: Target): boolean {
	if (
		target.url !== undefined ||
		target.state === "sent_as_draft" ||
		target.error !== undefined
	) {
		return true;
	}
	if (
		target.state === "waiting_approval" ||
		target.state === "scheduled" ||
		target.state === "approved"
	) {
		return true;
	}
	if (target.state === "draft" && target.scheduled_at === null) {
		return true;
	}
	return (
		target.state === "published" &&
		target.published_at !== undefined &&
		target.published_at !== ""
	);
}

// What a target came to: the address when it is published, the error and what to do about it when it failed,
// or a plain status outcome when none of those exist yet (screen 14c).
export function TargetResult({ target }: { target: Target }): ReactNode {
	const t = useT();
	const locale = useLocale();
	const link = linkOf(target.url);
	const hasExplicit =
		target.url !== undefined ||
		target.state === "sent_as_draft" ||
		target.error !== undefined;
	return (
		<>
			{target.url !== undefined &&
				(link === null ? (
					<UntrustedText>{target.url}</UntrustedText>
				) : (
					<a
						href={link}
						target="_blank"
						rel="noreferrer noopener"
						className="text-accent-text underline underline-offset-2"
						data-testid="target-url"
					>
						<UntrustedText>{target.url}</UntrustedText>
					</a>
				))}
			{target.state === "sent_as_draft" && (
				<span role="note" className="block text-sm" data-testid="target-draft">
					{t("ui.publish.draft_note")}
				</span>
			)}
			{target.error !== undefined && (
				<span role="note" className="block text-sm" data-testid="target-error">
					<span className="font-medium text-destructive">
						{t("ui.publish.error")}:{" "}
					</span>
					<UntrustedText>{target.error.message}</UntrustedText>
					{target.error.hint !== undefined && (
						<>
							{" "}
							<UntrustedText>{target.error.hint}</UntrustedText>
						</>
					)}
				</span>
			)}
			{!hasExplicit && target.state === "waiting_approval" && (
				<span className="text-xs text-warning-text">
					{t("ui.publish.detail.result_waiting")}
				</span>
			)}
			{!hasExplicit &&
				(target.state === "scheduled" || target.state === "approved") && (
					<span className="text-xs text-text-muted">
						{t("ui.publish.detail.result_queued")}
					</span>
				)}
			{!hasExplicit &&
				target.state === "draft" &&
				target.scheduled_at === null && (
					<span className="text-xs text-text-muted">
						{t("ui.publish.detail.result_needs_time")}
					</span>
				)}
			{!hasExplicit &&
				target.state === "published" &&
				target.published_at !== undefined &&
				target.published_at !== "" && (
					<span className="text-xs text-text-muted">
						{t("ui.publish.detail.result_published_at", {
							time: formatDateTime(target.published_at, locale),
						})}
					</span>
				)}
		</>
	);
}

// The list of the calendar: one row for each target, with the same facts as the calendar and a way to open it.
export function ListView({
	publications,
	accounts = {},
	onOpen,
	onInspect,
}: {
	// the name the person gave to each connection, by id: the list shows it, not the kind of the connection
	accounts?: Readonly<Record<string, string>>;
	// null while the first read has not answered or failed: a list not read is not an empty list
	publications: readonly Publication[] | null;
	onOpen(publicationId: string, targetId: string): void;
	onInspect?(publicationId: string, targetId: string): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	if (publications === null) return null;
	const rows = publications.flatMap((publication) =>
		publication.targets.map((target) => ({ publication, target })),
	);
	if (rows.length === 0) {
		return (
			<p className="text-sm text-muted-foreground" data-testid="publish-empty">
				{t("ui.publish.empty")}
			</p>
		);
	}
	return (
		<div
			className="overflow-x-auto rounded-lg border"
			data-testid="publish-list"
		>
			<table className="w-full text-sm">
				<caption className="sr-only">{t("ui.publish.list_caption")}</caption>
				<thead className="bg-muted/50 text-left text-xs">
					<tr>
						<th scope="col" className="px-3 py-2 font-medium">
							{t("ui.publish.col.when")}
						</th>
						<th scope="col" className="px-3 py-2 font-medium">
							{t("ui.publish.col.title")}
						</th>
						<th scope="col" className="px-3 py-2 font-medium">
							{t("ui.publish.col.account")}
						</th>
						<th scope="col" className="px-3 py-2 font-medium">
							{t("ui.publish.col.state")}
						</th>
						<th scope="col" className="px-3 py-2 font-medium">
							{t("ui.publish.col.result")}
						</th>
						<th scope="col" className="px-3 py-2">
							<span className="sr-only">{t("ui.publish.col.actions")}</span>
						</th>
					</tr>
				</thead>
				<tbody>
					{rows.map(({ publication, target }) => (
						<tr
							key={target.id}
							className="border-t align-top"
							data-testid="publish-row"
							data-target-id={target.id}
						>
							<td className="px-3 py-2 tabular-nums">
								{target.scheduled_at === null ? (
									t("ui.publish.no_time")
								) : (
									<time dateTime={target.scheduled_at}>
										{formatInZone(target.scheduled_at, target.zone, locale)}
									</time>
								)}
							</td>
							<td className="px-3 py-2">
								<UntrustedText>
									{target.fields.title || publication.title}
								</UntrustedText>
							</td>
							<td className="px-3 py-2">
								<UntrustedText>
									{accounts[target.connection_id] ?? target.network}
								</UntrustedText>
							</td>
							<td className="px-3 py-2">
								<TargetStateBadge state={target.state} />
							</td>
							<td className="px-3 py-2">
								<TargetResult target={target} />
							</td>
							<td className="px-3 py-2 text-right">
								<div className="inline-flex items-center justify-end gap-1.5">
									{onInspect !== undefined && (
										<Button
											type="button"
											variant="ghost"
											size="sm"
											data-testid="publish-details"
											onClick={() => onInspect(publication.id, target.id)}
										>
											{t("ui.publish.details")}
										</Button>
									)}
									<Button
										variant="outline"
										size="sm"
										onClick={() => onOpen(publication.id, target.id)}
									>
										{t("ui.publish.open")}
									</Button>
								</div>
							</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	);
}
