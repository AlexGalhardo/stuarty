import { Download } from "lucide-react";
import { type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { fileUrl } from "../api/imageApi";
import type { Finding, ImageExportResult } from "../api/types";
import { useImage, useImageApi, useImageStore } from "../state/context";
import ToolButton from "../toolbar/ToolButton";
import { ToolGroup } from "../toolbar/Toolbar";

const FORMATS = ["png", "jpeg", "webp"] as const;
type Format = (typeof FORMATS)[number];
// The names of the formats are the names of the files, the same in every language.
const FORMAT_NAME: Record<Format, string> = {
	png: "PNG",
	jpeg: "JPEG",
	webp: "WebP",
};
const SIZES = ["original", "video_thumbnail", "square_post", "vertical_cover"];

// A finding of error severity is the one that stops an export (SDD-021 section 8: only an empty slot is an
// error); the core still refuses it with E_LINT_BLOCKING, and this only says so before the call.
export function blocks(finding: Finding): boolean {
	return finding.severity === "error";
}

// The export dialog (SDD-012 section 24.8): the format, the size and the quality, the findings of the lint with
// a way to each layer, and the file when it is ready.
export function ExportTool(): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const [open, setOpen] = useState(false);

	return (
		<ToolGroup>
			<ToolButton
				icon={Download}
				label={t("ui.image.export.open")}
				disabled={doc === null}
				onClick={() => setOpen(true)}
				testId="image-export-open"
			/>
			{open && <ExportDialog onClose={() => setOpen(false)} />}
		</ToolGroup>
	);
}

function ExportDialog({ onClose }: { onClose(): void }): ReactNode {
	const t = useT();
	const api = useImageApi();
	const store = useImageStore();
	const doc = useImage((s) => s.doc);
	const [format, setFormat] = useState<Format>("png");
	const [size, setSize] = useState("original");
	const [quality, setQuality] = useState("90");
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [result, setResult] = useState<ImageExportResult | null>(null);

	if (doc === null) return null;
	const findings = doc.findings;
	const blocked = findings.filter(blocks);

	const run = async (): Promise<void> => {
		setBusy(true);
		setFailure(null);
		setResult(null);
		try {
			const q = Number(quality);
			setResult(
				await api.exportImage(doc.image_id, {
					format,
					...(size === "original" ? {} : { preset: size }),
					...(format === "jpeg" && Number.isFinite(q) ? { quality: q } : {}),
				}),
			);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	};

	const goTo = (finding: Finding): void => {
		if (finding.item !== undefined) store.getState().select([finding.item]);
		onClose();
	};

	return (
		<Dialog open onOpenChange={(next) => !next && onClose()}>
			<DialogContent data-testid="image-export-dialog">
				<DialogTitle>{t("ui.image.export.title")}</DialogTitle>
				<DialogDescription>{t("ui.image.export.hint")}</DialogDescription>

				<fieldset className="flex flex-col gap-1 border-0 p-0">
					<legend className="mb-1 text-sm font-medium">
						{t("ui.image.export.format")}
					</legend>
					<div className="flex gap-4">
						{FORMATS.map((f) => (
							<label key={f} className="flex items-center gap-1 text-sm">
								<input
									type="radio"
									name="image-export-format"
									value={f}
									checked={format === f}
									data-testid={`image-export-format-${f}`}
									onChange={() => setFormat(f)}
								/>
								{FORMAT_NAME[f]}
							</label>
						))}
					</div>
				</fieldset>

				<div className="flex flex-wrap items-end gap-3">
					<div className="flex flex-col gap-1">
						<Label htmlFor="image-export-size">
							{t("ui.image.export.size")}
						</Label>
						<select
							id="image-export-size"
							data-testid="image-export-size"
							value={size}
							onChange={(event) => setSize(event.target.value)}
							className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
						>
							{SIZES.map((s) => (
								<option key={s} value={s}>
									{s === "original"
										? t("ui.image.export.original", {
												w: doc.width,
												h: doc.height,
											})
										: t(`ui.image.preset.${s}`)}
								</option>
							))}
						</select>
					</div>
					{format === "jpeg" && (
						<div className="flex w-24 flex-col gap-1">
							<Label htmlFor="image-export-quality">
								{t("ui.image.export.quality")}
							</Label>
							<Input
								id="image-export-quality"
								data-testid="image-export-quality"
								inputMode="numeric"
								value={quality}
								onChange={(event) => setQuality(event.target.value)}
							/>
						</div>
					)}
				</div>

				<section
					aria-labelledby="image-export-findings"
					data-testid="image-export-findings"
				>
					<h3 id="image-export-findings" className="mb-1 text-sm font-medium">
						{t("ui.image.export.findings")}
					</h3>
					{findings.length === 0 ? (
						<p className="text-sm text-muted-foreground">
							{t("ui.image.export.no_findings")}
						</p>
					) : (
						<ul className="flex max-h-40 flex-col gap-1 overflow-y-auto">
							{findings.map((finding) => (
								<li
									key={`${finding.code}-${finding.item ?? ""}`}
									className="flex items-start justify-between gap-2 rounded border p-2 text-sm"
								>
									<span className="min-w-0">
										<strong>
											{t(
												blocks(finding)
													? "ui.image.export.error"
													: "ui.image.export.warning",
											)}
										</strong>
										{": "}
										<span lang="en">
											<UntrustedText>{finding.message}</UntrustedText>
										</span>
									</span>
									{finding.item !== undefined && (
										<Button
											type="button"
											variant="outline"
											size="xs"
											data-testid="image-finding-go"
											onClick={() => goTo(finding)}
										>
											{t("ui.image.export.go_to_layer")}
										</Button>
									)}
								</li>
							))}
						</ul>
					)}
					{blocked.length > 0 && (
						<p role="alert" className="mt-1 text-sm text-destructive">
							{t("ui.image.export.blocked", { count: blocked.length })}
						</p>
					)}
				</section>

				<FailureNote failure={failure} />

				{result !== null && (
					<div
						data-testid="image-export-result"
						className="flex flex-col gap-1 rounded border p-2 text-sm"
					>
						<p>
							{t("ui.image.export.done", {
								w: result.width,
								h: result.height,
								bytes: result.bytes,
							})}
						</p>
						<p className="break-all font-mono text-xs text-muted-foreground">
							{t("ui.image.export.digest", { hash: result.sha256 })}
						</p>
						<Button asChild variant="outline" size="sm">
							<a
								href={fileUrl(result.image_id, result.path)}
								download
								data-testid="image-export-download"
							>
								<Download aria-hidden="true" />
								{t("ui.image.export.download")}
							</a>
						</Button>
					</div>
				)}

				<div className="flex justify-end gap-2">
					<Button type="button" variant="outline" onClick={onClose}>
						{t("ui.image.export.close")}
					</Button>
					<Button
						type="button"
						disabled={busy || blocked.length > 0}
						data-testid="image-export-run"
						onClick={() => void run()}
					>
						{t(busy ? "ui.image.export.exporting" : "ui.image.export.run")}
					</Button>
				</div>
			</DialogContent>
		</Dialog>
	);
}
