import { CircleAlert, Copy } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Progress } from "@/components/ui/progress";
import { useLocale, useT } from "@/lib/i18n";
import { ApiError } from "../../api/errors";
import type { ToolOffer } from "../../api/types";
import ErrorBody from "../../commands/ErrorBody";
import { useRuntime } from "../../state/EditorContext";
import { errorHeadline } from "../../ui/errorHeadline";
import { formatDecimal } from "../../ui/format";
import { FACE_SET } from "./verticalCutsModel";

const POLL_EVERY_MS = 1000;

type Phase =
	| { name: "offer" }
	| { name: "installing"; done: number; total: number }
	| { name: "done" }
	| { name: "command_only" }
	| { name: "failed"; error: { code: string; message: string; hint?: string } };

const megabytes = (bytes: number, locale: string): string =>
	formatDecimal(bytes / 1_000_000, locale);

// The install prompt of the face tracking set (SDD-024 section 3). It states what would be downloaded, from
// where, how big it is, under which licences and what the model was trained on, and downloads nothing until
// the person presses Install: that click sends the one install request. Cancel sends none.
export default function FaceInstallDialog({
	offer,
	onClose,
	onInstalled,
}: {
	offer: ToolOffer;
	onClose(): void;
	onInstalled(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const runtime = useRuntime();
	const [phase, setPhase] = useState<Phase>({ name: "offer" });
	const command = `stuarty tools:install ${FACE_SET}`;

	const installing = phase.name === "installing";
	useEffect(() => {
		if (!installing) return;
		const timer = setInterval(() => {
			runtime.client.getTools().then(
				(status) => {
					if (status.state === "running") {
						setPhase({
							name: "installing",
							done: status.done_bytes,
							total: status.total_bytes,
						});
					} else if (status.state === "failed") {
						setPhase({
							name: "failed",
							error: status.error ?? {
								code: "E_TRANSCRIBE_UNAVAILABLE",
								message: status.text,
							},
						});
					} else {
						setPhase({ name: "done" });
						onInstalled();
					}
				},
				() => undefined,
			);
		}, POLL_EVERY_MS);
		return () => clearInterval(timer);
	}, [installing, runtime, onInstalled]);

	async function install(): Promise<void> {
		setPhase({ name: "installing", done: 0, total: offer.download_bytes });
		try {
			await runtime.client.installTool({ name: FACE_SET });
		} catch (error) {
			if (!(error instanceof ApiError)) throw error;
			if (error.code === "E_FORBIDDEN") {
				setPhase({ name: "command_only" });
				return;
			}
			setPhase({
				name: "failed",
				error: { code: error.code, message: error.message, hint: error.hint },
			});
		}
	}

	const percent =
		phase.name === "installing" && phase.total > 0
			? Math.min(100, Math.round((phase.done / phase.total) * 100))
			: 0;

	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{t("ui.editor.vertical_cuts.install.title")}
					</DialogTitle>
					<DialogDescription>
						{t("ui.editor.vertical_cuts.install.description")}
					</DialogDescription>
				</DialogHeader>

				<div className="flex flex-col gap-3 text-sm">
					<ul
						className="list-disc pl-5 text-xs"
						data-testid="face-install-files"
					>
						{offer.files.map((file) => (
							<li key={file.name}>
								<span className="font-mono">{file.name}</span>
								{` · ${megabytes(file.bytes, locale)} MB · `}
								{t("ui.editor.vertical_cuts.install.source", {
									host: file.host,
								})}
								{file.license !== undefined &&
									` · ${t("ui.editor.vertical_cuts.install.license", { license: file.license })}`}
							</li>
						))}
					</ul>
					<p data-testid="face-install-size">
						{t("ui.editor.vertical_cuts.install.size", {
							megabytes: megabytes(offer.download_bytes, locale),
						})}
					</p>
					{offer.dataset !== undefined && (
						<p className="text-xs text-muted-foreground">
							{t("ui.editor.vertical_cuts.install.dataset")}{" "}
							<UntrustedText>{offer.dataset}</UntrustedText>
						</p>
					)}
					{!offer.published && (
						<p role="status" className="text-xs text-destructive">
							{t("ui.editor.vertical_cuts.install.not_published")}
						</p>
					)}

					{phase.name === "installing" && (
						<div>
							<Progress
								value={percent}
								aria-label={t("ui.editor.vertical_cuts.install.progress")}
								className="h-2"
							/>
							<p className="mt-1 text-xs [font-variant-numeric:tabular-nums]">
								{t("ui.editor.vertical_cuts.install.installing", {
									done: megabytes(phase.done, locale),
									total: megabytes(phase.total, locale),
								})}
							</p>
						</div>
					)}
					{phase.name === "done" && (
						<p role="status">{t("ui.editor.vertical_cuts.install.done")}</p>
					)}
					{phase.name === "command_only" && (
						<p role="status" className="text-muted-foreground">
							{t("ui.editor.vertical_cuts.install.forbidden")}
						</p>
					)}
					{phase.name === "failed" && (
						<div role="alert">
							<p className="flex items-center gap-1.5 font-medium text-destructive">
								<CircleAlert aria-hidden="true" className="size-4" />
								{errorHeadline(t, phase.error.code)}
							</p>
							<ErrorBody error={phase.error} />
						</div>
					)}

					<div>
						<p className="text-xs">
							{t("ui.editor.vertical_cuts.install.command")}
						</p>
						<p className="flex items-center gap-1">
							<input
								readOnly
								value={command}
								aria-label={t("ui.editor.vertical_cuts.install.command")}
								className="h-8 min-w-0 flex-1 rounded-md border bg-transparent px-2 font-mono text-xs"
								onFocus={(event) => event.currentTarget.select()}
							/>
							<Button
								type="button"
								variant="ghost"
								size="icon-sm"
								aria-label={t("ui.editor.vertical_cuts.install.copy_command")}
								onClick={() => void navigator.clipboard.writeText(command)}
							>
								<Copy aria-hidden="true" />
							</Button>
						</p>
					</div>
				</div>

				<DialogFooter>
					{phase.name === "done" || phase.name === "command_only" ? (
						<Button type="button" onClick={onClose}>
							{t("ui.editor.vertical_cuts.install.close")}
						</Button>
					) : (
						<>
							<Button type="button" variant="outline" onClick={onClose}>
								{t("ui.editor.vertical_cuts.install.cancel")}
							</Button>
							<Button
								type="button"
								disabled={phase.name === "installing" || !offer.published}
								onClick={() => void install()}
							>
								{t("ui.editor.vertical_cuts.install.install")}
							</Button>
						</>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
