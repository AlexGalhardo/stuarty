import { type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Badge } from "@/components/ui/badge";
import { useT } from "@/lib/i18n";
import type { CaptionValidateResult, WritingClient } from "./client";
import { keyed } from "./keyed";
import { CHECK_DELAY_MS } from "./ScriptPanel";

const KNOWN_UNITS = ["bytes", "characters", "utf16_units", "weighted"];

// The counters of a caption against the limits of each network (SDD-019 sections 5 and 11). The whole body goes to
// the service, which splits it into the variants and measures each one: this panel draws the counts and the
// problems it answers, the sentence of each problem included.
export default function CaptionPanel({
	client,
	projectId,
	body,
}: {
	client: WritingClient;
	projectId: string;
	body: string;
}): ReactNode {
	const t = useT();
	const [report, setReport] = useState<CaptionValidateResult | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	useEffect(() => {
		if (body.trim() === "") {
			setReport(null);
			setFailure(null);
			return;
		}
		let live = true;
		const timer = window.setTimeout(() => {
			client
				.validateCaption(body, projectId)
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
		}, CHECK_DELAY_MS);
		return () => {
			live = false;
			window.clearTimeout(timer);
		};
	}, [client, projectId, body]);

	return (
		<section
			aria-labelledby="caption-heading"
			className="flex flex-col gap-2"
			data-testid="caption-panel"
		>
			<h3 id="caption-heading" className="font-medium">
				{t("ui.write.caption.heading")}
			</h3>
			<p className="text-xs text-muted-foreground">
				{t("ui.write.caption.hint")}
			</p>
			<FailureNote failure={failure} />
			<div role="status" aria-live="polite" className="flex flex-col gap-3">
				{report === null && failure === null && (
					<p className="text-sm text-muted-foreground">
						{t("ui.write.caption.empty")}
					</p>
				)}
				{keyed(report?.variants ?? [], (v) => v.network).map(
					({ key, item: variant }) => {
						const unit =
							variant.unit !== undefined && KNOWN_UNITS.includes(variant.unit)
								? t(`ui.write.caption.unit.${variant.unit}`)
								: (variant.unit ?? "");
						const name =
							variant.network === ""
								? t("ui.write.caption.common")
								: (variant.name ?? variant.network);
						const length = String(variant.counts.length);
						return (
							<div
								key={key}
								data-network={
									variant.network === "" ? "common" : variant.network
								}
								className="flex flex-col gap-1 rounded-lg border p-2 text-sm"
							>
								<div className="flex flex-wrap items-center gap-2">
									<span className="font-medium">{name}</span>
									<Badge variant={variant.ok ? "secondary" : "destructive"}>
										{variant.ok
											? t("ui.write.caption.ok")
											: variant.problems[0]?.code}
									</Badge>
								</div>
								<p data-testid="caption-count">
									{variant.max_length > 0
										? t("ui.write.caption.count", {
												length,
												max: String(variant.max_length),
												unit,
											})
										: t("ui.write.caption.count_free", { length, unit })}
									{" · "}
									{t("ui.write.caption.hashtags", {
										count: String(variant.counts.hashtags),
									})}
									{" · "}
									{t("ui.write.caption.mentions", {
										count: String(variant.counts.mentions),
									})}
									{" · "}
									{t("ui.write.caption.links", {
										count: String(variant.counts.links),
									})}
								</p>
								{variant.counts.links > 0 && !variant.links_clickable && (
									<p className="text-muted-foreground">
										{t("ui.write.caption.links_dead")}
									</p>
								)}
								{variant.problems.length > 0 && (
									<ul className="list-disc pl-5 text-destructive">
										{variant.problems.map((problem) => (
											<li key={problem.code} lang="en">
												{problem.message}
											</li>
										))}
									</ul>
								)}
							</div>
						);
					},
				)}
			</div>
		</section>
	);
}
