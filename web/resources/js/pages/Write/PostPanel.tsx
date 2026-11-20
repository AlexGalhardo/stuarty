import { type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { useT } from "@/lib/i18n";
import type { PostCheckResult, WritingClient } from "./client";
import { keyed } from "./keyed";
import MarkdownPreview from "./Preview";

// The check of a blog post (SDD-019 sections 11 and 13): the problems the core found in the saved text, the images it
// uses and how the body reads. The check runs on the saved revision, so it is read again when the revision changes.
// The page draws what the service answered: it parses no front matter and renders no HTML.
export default function PostPanel({
	client,
	projectId,
	itemId,
	revision,
	dirty,
}: {
	client: WritingClient;
	projectId: string;
	itemId: string;
	revision: number;
	dirty: boolean;
}): ReactNode {
	const t = useT();
	const [report, setReport] = useState<PostCheckResult | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	// biome-ignore lint/correctness/useExhaustiveDependencies: a new revision is the reason to ask again
	useEffect(() => {
		let live = true;
		client
			.checkPost(projectId, itemId)
			.then((result) => {
				if (!live) return;
				setReport(result);
				setFailure(null);
			})
			.catch((error: unknown) => {
				if (!live) return;
				setReport(null);
				setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, [client, projectId, itemId, revision]);

	return (
		<section
			aria-labelledby="post-heading"
			className="flex flex-col gap-3"
			data-testid="post-panel"
		>
			<h3 id="post-heading" className="font-medium">
				{t("ui.write.post.heading")}
			</h3>
			<p className="text-xs text-muted-foreground">{t("ui.write.post.hint")}</p>
			{dirty && (
				<p role="status" className="text-xs text-muted-foreground">
					{t("ui.write.post.unsaved")}
				</p>
			)}
			<FailureNote failure={failure} />
			<div role="status" aria-live="polite" className="flex flex-col gap-3">
				{report === null && failure === null && (
					<p className="text-sm text-muted-foreground">
						{t("ui.write.post.empty")}
					</p>
				)}
				{report !== null && (
					<>
						<div className="flex flex-wrap items-center gap-2 text-sm">
							<Badge variant={report.ok ? "secondary" : "destructive"}>
								{report.ok
									? t("ui.write.post.ok")
									: t("ui.write.post.issues", {
											count: String(report.issues.length),
										})}
							</Badge>
							<span data-testid="post-slug">
								<UntrustedText>
									{t("ui.write.post.slug", { slug: report.slug })}
								</UntrustedText>
							</span>
							<span>
								{t("ui.write.post.words", { words: String(report.words) })}
							</span>
						</div>
						{report.issues.length > 0 && (
							<div>
								<h4 className="text-sm font-medium">
									{t("ui.write.post.issues_heading")}
								</h4>
								<ul
									className="list-disc pl-5 text-sm text-destructive"
									data-testid="post-issues"
								>
									{keyed(report.issues, (i) => `${i.path}:${i.code}`).map(
										({ key, item }) => (
											<li key={key} data-code={item.code} lang="en">
												{item.path}: {item.message}
											</li>
										),
									)}
								</ul>
							</div>
						)}
						<div>
							<h4 className="text-sm font-medium">
								{t("ui.write.post.images_heading")}
							</h4>
							{report.images.length === 0 ? (
								<p className="text-sm text-muted-foreground">
									{t("ui.write.post.images_none")}
								</p>
							) : (
								<ul
									className="list-disc pl-5 text-sm"
									data-testid="post-images"
								>
									{keyed(report.images, (i) => i.asset_id).map(
										({ key, item }) => (
											<li key={key} data-found={item.found ? "yes" : "no"}>
												{item.found ? (
													<UntrustedText>
														{t("ui.write.post.image_row", {
															name: item.name,
															role: t(`ui.write.post.role_${item.role}`),
															bytes: String(item.bytes ?? 0),
														})}
													</UntrustedText>
												) : (
													<span className="text-destructive">
														{t("ui.write.post.image_missing", {
															id: item.asset_id,
														})}
													</span>
												)}
											</li>
										),
									)}
								</ul>
							)}
						</div>
						<div>
							<h4 className="text-sm font-medium">
								{t("ui.write.post.preview_heading")}
							</h4>
							<MarkdownPreview
								text={report.markdown}
								label={t("ui.write.post.preview_label")}
								empty={t("ui.write.post.preview_empty")}
							/>
						</div>
					</>
				)}
			</div>
			<p className="text-xs text-muted-foreground">
				{t("ui.write.post.publish_hint")}
			</p>
		</section>
	);
}
