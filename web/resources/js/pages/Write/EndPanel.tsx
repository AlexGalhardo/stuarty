import { type ReactNode, useState } from "react";

import { Notice } from "@/components/Notice";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { TextKind } from "./client";
import { type End, hrefOfStep } from "./meta";
import Panel from "./Panel";

export type EndStatus =
	| { state: "copied" }
	| { state: "exported"; file: string }
	| { state: "failed" };

// The end of a text (SDD-019 section 11, D-137). A copy ends in Copy and Export: no destination is asked for, and
// sending it to a blog is a choice the person makes by opening it. Any other text offers the next step of the path
// the service listed. Copy and Export act on the saved text, so they wait while there are unsaved changes.
export default function EndPanel({
	kind,
	end,
	projectId,
	dirty,
	reviewed,
	status,
	onCopy,
	onExport,
}: {
	kind: TextKind;
	end: End;
	projectId: string;
	dirty: boolean;
	reviewed: boolean;
	status: EndStatus | null;
	onCopy: () => void;
	onExport: () => void;
}): ReactNode {
	const t = useT();
	const [blog, setBlog] = useState(false);
	const primaryCopy = reviewed && !dirty && end.kind === "copy";
	return (
		<Panel
			title={t("ui.write.end.heading")}
			description={t(`ui.write.end.intro.${kind}`)}
			testId="end-panel"
			headingId="end-heading"
		>
			<div className="flex flex-wrap items-center gap-2">
				<Button
					type="button"
					variant={primaryCopy ? "default" : "outline"}
					data-primary={primaryCopy ? "true" : undefined}
					disabled={dirty}
					onClick={onCopy}
				>
					{t("ui.write.end.copy")}
				</Button>
				<Button
					type="button"
					variant="outline"
					disabled={dirty}
					onClick={onExport}
				>
					{t("ui.write.end.export")}
				</Button>
				{end.kind === "step" && (
					<Button
						asChild
						variant={reviewed && !dirty ? "default" : "outline"}
						data-primary={reviewed && !dirty ? "true" : undefined}
					>
						<a href={hrefOfStep(end.step, projectId)}>
							{t("ui.write.end.next", {
								step: t(`ui.write.flow.step.${end.step.id}`),
							})}
						</a>
					</Button>
				)}
			</div>
			{dirty && (
				<p className="text-sm text-text-muted">{t("ui.write.end.unsaved")}</p>
			)}
			<div role="status" aria-live="polite" data-testid="end-status">
				{status?.state === "copied" && (
					<p className="text-sm">{t("ui.write.end.copied")}</p>
				)}
				{status?.state === "exported" && (
					<p className="text-sm">
						{t("ui.write.end.exported", { file: status.file })}
					</p>
				)}
			</div>
			{status?.state === "failed" && (
				<p role="alert" className="text-sm text-danger-text">
					{t("ui.write.end.failed")}
				</p>
			)}
			<p className="text-sm text-text-muted">
				{t("ui.write.end.no_destination")}
			</p>
			{kind === "copy" && (
				<div className="flex flex-col gap-2 border-t border-border pt-3">
					<div>
						<Button
							type="button"
							variant="ghost"
							aria-expanded={blog}
							aria-controls="end-blog"
							onClick={() => setBlog((open) => !open)}
						>
							{t("ui.write.end.blog")}
						</Button>
					</div>
					{blog && (
						<div id="end-blog">
							<Notice
								tone="info"
								title={t("ui.write.end.blog_title")}
								data-testid="end-blog"
								actions={
									<>
										<a className="underline" href="/connections">
											{t("ui.write.end.blog_connections")}
										</a>
										<a className="underline" href="/publish">
											{t("ui.write.end.blog_publish")}
										</a>
									</>
								}
							>
								{t("ui.write.end.blog_body")}
							</Notice>
						</div>
					)}
				</div>
			)}
		</Panel>
	);
}
