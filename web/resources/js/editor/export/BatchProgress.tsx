import {
	CircleAlert,
	CircleCheck,
	CircleSlash,
	Clock,
	Copy,
	LoaderCircle,
} from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { useLocale, useT } from "@/lib/i18n";
import type { BatchItem, BatchResult, ExportPreset } from "../api/types";
import { exportFileUrl } from "../api/urls";
import ErrorBody from "../commands/ErrorBody";
import { formatDecimal } from "../ui/format";
import { itemName } from "./presetText";

function StateIcon({ state }: { state: BatchItem["state"] }): ReactNode {
	const common = { "aria-hidden": true, className: "size-4 shrink-0" } as const;
	switch (state) {
		case "succeeded":
			return (
				<CircleCheck {...common} className="size-4 shrink-0 text-success" />
			);
		case "failed":
			return (
				<CircleAlert {...common} className="size-4 shrink-0 text-destructive" />
			);
		case "cancelled":
			return <CircleSlash {...common} />;
		case "running":
			return <LoaderCircle {...common} />;
		default:
			return <Clock {...common} />;
	}
}

// The exports of a batch (RF-088), one row each, from the children of the batch
// job: where it stands, the file with its download, or the error with its hint.
// The core runs them one after the other, so a failed row never stops the next.
export default function BatchProgress({
	batch,
	presets,
}: {
	batch: BatchResult;
	presets: readonly ExportPreset[];
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	return (
		<section data-testid="batch-progress" className="mt-2">
			<h3 className="text-sm font-semibold">
				{t("ui.editor.export.batch.title")}{" "}
				<span className="font-normal text-muted-foreground [font-variant-numeric:tabular-nums]">
					{t("ui.editor.export.batch.summary", {
						done: batch.succeeded,
						total: batch.total,
					})}
				</span>
			</h3>
			<ol className="mt-1 flex flex-col gap-2">
				{batch.items.map((item) => {
					const name = itemName(item, presets, t);
					const result = item.result;
					return (
						<li
							key={item.index}
							data-batch-item={item.index}
							data-state={item.state}
							className="text-sm"
						>
							<p className="flex items-center gap-1.5">
								<StateIcon state={item.state} />
								<span className="font-medium">
									{t("ui.editor.export.batch.item", {
										index: item.index + 1,
										name,
									})}
								</span>
								<span className="text-xs text-muted-foreground">
									{t(`ui.editor.export.batch.state.${item.state}`)}
								</span>
							</p>
							{item.state === "running" && (
								<Progress
									value={Math.round(item.progress * 100)}
									aria-label={name}
									className="my-1 h-1.5"
								/>
							)}
							{result !== undefined && (
								<div className="pl-6 text-xs">
									{result.path !== undefined && (
										<p className="flex items-center gap-1 font-mono break-all">
											<UntrustedText>{result.path}</UntrustedText>
											<Button
												type="button"
												variant="ghost"
												size="icon-sm"
												aria-label={t("ui.editor.export.copy_path")}
												onClick={() =>
													void navigator.clipboard.writeText(result.path ?? "")
												}
											>
												<Copy aria-hidden="true" />
											</Button>
										</p>
									)}
									<p className="text-muted-foreground [font-variant-numeric:tabular-nums]">
										{t("ui.editor.export.batch.codec_note", {
											codec: result.video_codec ?? "",
											size: formatDecimal(result.bytes / 1_000_000, locale),
										})}
									</p>
									{(result.notes ?? []).map((note) => (
										<p
											key={note}
											role="status"
											lang="en"
											className="text-warning"
										>
											<UntrustedText>{note}</UntrustedText>
										</p>
									))}
									{item.job_id !== undefined && (
										<Button asChild variant="link" size="sm">
											<a href={exportFileUrl(item.job_id)} download>
												{t("ui.editor.export.batch.download", { name })}
											</a>
										</Button>
									)}
								</div>
							)}
							{item.error !== undefined && item.state !== "cancelled" && (
								<div role="alert" className="pl-6 text-xs">
									<ErrorBody error={item.error} />
									<p className="font-mono">{item.error.code}</p>
								</div>
							)}
						</li>
					);
				})}
			</ol>
		</section>
	);
}
