import { CheckCircle2, Circle, Compass, ExternalLink } from "lucide-react";
import type { ReactNode } from "react";

import { Notice } from "@/components/Notice";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { formatDateTime, formatInZone } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import type { Workflow } from "../../workflow/types";
import { stateWord, titleWord } from "../../workflow/words";
import { linkOf } from "./publicationState";
import type { Publication, Target } from "./types";

// Finds the most recently published blog post target from a publications list returned by the service.
export function latestPublishedPost(
	publications: readonly Publication[],
): { publication: Publication; target: Target } | null {
	const matches: { publication: Publication; target: Target }[] = [];
	for (const publication of publications) {
		if ((publication.post ?? "") === "") continue;
		for (const target of publication.targets) {
			if (target.state === "published") {
				matches.push({ publication, target });
			}
		}
	}
	if (matches.length === 0) return null;
	matches.sort((a, b) =>
		(b.target.published_at ?? b.target.updated_at).localeCompare(
			a.target.published_at ?? a.target.updated_at,
		),
	);
	return matches[0] ?? null;
}

// The published completion state of a text project (screen 33h): the success notice, the completed trail
// when the server's workflow is present, and the optional "Add a video" action that keeps everything done.
export function PublishedPostState({
	title,
	destination,
	publishedAt,
	zone,
	url,
	workflow,
	addVideoAction,
}: {
	title: string;
	destination?: string;
	publishedAt?: string | null;
	zone?: string;
	url?: string;
	workflow?: Workflow | null;
	addVideoAction: ReactNode;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const safeLink = linkOf(url);
	const formattedTime =
		publishedAt != null && publishedAt !== ""
			? zone
				? formatInZone(publishedAt, zone, locale)
				: formatDateTime(publishedAt, locale)
			: null;

	return (
		<section
			aria-label={t("ui.publish.published_state.title")}
			data-testid="published-post-state"
			className="flex flex-col gap-4"
		>
			<Notice
				tone="success"
				role="status"
				data-testid="published-banner"
				title={t("ui.publish.published_state.title")}
				actions={
					safeLink !== null ? (
						<Button asChild variant="outline" size="sm">
							<a
								href={safeLink}
								target="_blank"
								rel="noreferrer noopener"
								data-testid="published-open-post"
							>
								<ExternalLink aria-hidden="true" />
								{t("ui.publish.published_state.open_post")}
							</a>
						</Button>
					) : undefined
				}
			>
				<p>
					{destination && formattedTime ? (
						<UntrustedText>
							{t("ui.publish.published_state.body_with_place", {
								title,
								destination,
								time: formattedTime,
							})}
						</UntrustedText>
					) : (
						<UntrustedText>
							{t("ui.publish.published_state.body_plain", { title })}
						</UntrustedText>
					)}
				</p>
			</Notice>

			<div
				className={
					workflow && workflow.steps.length > 0
						? "grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]"
						: "flex flex-col gap-4"
				}
			>
				<div
					data-testid="published-add-video"
					className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-border bg-surface p-4"
				>
					<div className="flex min-w-0 flex-1 items-start gap-3">
						<span
							aria-hidden="true"
							className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent-text"
						>
							<Compass className="size-4" />
						</span>
						<div className="min-w-0">
							<h3 className="text-sm font-semibold text-accent-text">
								{t("ui.publish.published_state.optional_title")}
							</h3>
							<p className="mt-0.5 text-sm text-text-muted">
								{t("ui.publish.published_state.optional_body")}
							</p>
						</div>
					</div>
					<div className="shrink-0">{addVideoAction}</div>
				</div>

				{workflow && workflow.steps.length > 0 && (
					<div
						data-testid="completed-trail"
						className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-4"
					>
						<h3 className="text-base font-semibold text-text">
							{t("ui.publish.published_state.trail_title")}
						</h3>
						<ol className="flex flex-col gap-2 text-sm">
							{workflow.steps.map((step) => (
								<li
									key={step.id}
									data-completed-step={step.id}
									data-state={step.state}
									className="flex items-center justify-between gap-3"
								>
									<span className="flex items-center gap-2 font-medium text-text">
										{step.state === "done" ? (
											<CheckCircle2
												aria-hidden="true"
												className="size-4 shrink-0 text-success-text"
											/>
										) : (
											<Circle
												aria-hidden="true"
												className="size-4 shrink-0 text-text-subtle"
											/>
										)}
										{titleWord(t, step)}
									</span>
									<span className="text-xs text-text-muted">
										{stateWord(t, step)}
									</span>
								</li>
							))}
						</ol>
					</div>
				)}
			</div>
		</section>
	);
}
