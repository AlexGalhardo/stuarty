import { Link } from "@inertiajs/react";
import { ExternalLink, Pencil, X } from "lucide-react";
import type { ReactNode } from "react";

import { DetailPanel } from "@/components/DetailPanel";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { formatDateTime, formatInZone } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { TargetResult } from "./ListView";
import { linkOf } from "./publicationState";
import { TargetStateBadge } from "./TargetStateBadge";
import type { Publication, Target } from "./types";

// Structured read-only details of one publication target in /publish: destination, status, scheduled/published
// times, and result (published URL or failure reason). Every fact comes directly from the service.
export function PublicationDetailPanel({
	publication,
	target,
	accounts = {},
	onEdit,
	onClose,
}: {
	publication: Publication;
	target: Target;
	accounts?: Readonly<Record<string, string>>;
	onEdit(): void;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const isPost = (publication.post ?? "") !== "";
	const destination = accounts[target.connection_id] ?? target.network;
	const safeLink = linkOf(target.url);
	const hasExplicitResult =
		target.url !== undefined ||
		target.state === "sent_as_draft" ||
		target.error !== undefined;

	return (
		<div data-testid="publication-detail">
			<DetailPanel
				title={
					<UntrustedText>
						{target.fields.title || publication.title}
					</UntrustedText>
				}
				actions={
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						aria-label={t("ui.publish.detail.close")}
						onClick={onClose}
						data-testid="publication-detail-close"
					>
						<X aria-hidden="true" />
					</Button>
				}
			>
				<div className="flex flex-wrap items-center gap-2">
					<StatusBadge status="neutral">
						{t(
							isPost
								? "ui.publish.editor.content_post"
								: "ui.publish.editor.content_video",
						)}
					</StatusBadge>
					<TargetStateBadge state={target.state} />
				</div>
				<dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 text-sm">
					<dt className="text-text-muted">
						{t("ui.publish.detail.destination")}
					</dt>
					<dd data-testid="detail-destination" className="min-w-0 break-words">
						<UntrustedText>{destination}</UntrustedText>
					</dd>
					<dt className="text-text-muted">{t("ui.publish.detail.status")}</dt>
					<dd data-testid="detail-status">
						{t(`ui.publish.state.${target.state}`)}
					</dd>
					<dt className="text-text-muted">
						{t("ui.publish.detail.scheduled")}
					</dt>
					<dd data-testid="detail-scheduled" className="tabular-nums">
						{target.scheduled_at === null ? (
							t("ui.publish.no_time")
						) : (
							<time dateTime={target.scheduled_at}>
								{formatInZone(target.scheduled_at, target.zone, locale)}
							</time>
						)}
					</dd>
					{target.published_at !== undefined && (
						<>
							<dt className="text-text-muted">
								{t("ui.publish.detail.published_at")}
							</dt>
							<dd data-testid="detail-published-at" className="tabular-nums">
								<time dateTime={target.published_at}>
									{formatDateTime(target.published_at, locale)}
								</time>
							</dd>
						</>
					)}
					<dt className="text-text-muted">{t("ui.publish.detail.result")}</dt>
					<dd data-testid="detail-result" className="min-w-0 break-words">
						{hasExplicitResult ? (
							<TargetResult target={target} />
						) : (
							<span className="text-text-muted">
								{t("ui.publish.detail.no_result")}
							</span>
						)}
					</dd>
				</dl>
				{target.state === "waiting_approval" && (
					<p className="text-xs text-text-muted">
						{t("ui.publish.editor.waits")}{" "}
						<Link
							href="/approvals"
							className="font-medium text-accent-text underline underline-offset-2"
						>
							{t("ui.social.open_approvals")}
						</Link>
					</p>
				)}
				<div className="flex flex-wrap items-center gap-2 pt-1">
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={onEdit}
						data-testid="publication-detail-edit"
					>
						<Pencil aria-hidden="true" />
						{t("ui.publish.detail.edit")}
					</Button>
					{safeLink !== null && (
						<Button asChild variant="outline" size="sm">
							<a
								href={safeLink}
								target="_blank"
								rel="noreferrer noopener"
								data-testid="publication-detail-open-post"
							>
								<ExternalLink aria-hidden="true" />
								{t("ui.publish.detail.open_post")}
							</a>
						</Button>
					)}
				</div>
			</DetailPanel>
		</div>
	);
}
