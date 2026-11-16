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
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { useLocale, useT } from "@/lib/i18n";
import { ApiError } from "../../api/errors";
import { useRuntime } from "../../state/EditorContext";
import { errorHeadline } from "../../ui/errorHeadline";
import { formatDecimal } from "../../ui/format";
import {
	DEFAULT_WHISPER_MODEL,
	installRequest,
	type Unavailable,
	WHISPER_MODELS,
} from "./transcriptModel";

const POLL_EVERY_MS = 1000;

type Phase =
	| { name: "offer" }
	| { name: "installing"; done: number; total: number }
	| { name: "done" }
	| { name: "command_only" }
	| {
			name: "failed";
			error: { code: string; message: string; hint?: string };
	  };

const megabytes = (bytes: number, locale: string): string =>
	formatDecimal(bytes / 1_000_000, locale);

// The install dialog of SDD-012 section 12.5 (RF-205). It states what is
// missing, its size and its source, and downloads nothing until the person
// presses Install: that click sends the one install request. Cancel sends none.
export default function InstallDialog({
	unavailable,
	onClose,
}: {
	unavailable: Unavailable | null;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const runtime = useRuntime();
	const [phase, setPhase] = useState<Phase>({ name: "offer" });
	const [model, setModel] = useState(DEFAULT_WHISPER_MODEL);
	const base = unavailable === null ? null : installRequest(unavailable);
	const choosesModel = base?.name === "whisper";
	const request = base !== null && choosesModel ? { ...base, model } : base;

	// A new answer of the service starts the dialog again.
	// biome-ignore lint/correctness/useExhaustiveDependencies: `unavailable` is the trigger.
	useEffect(() => {
		setPhase({ name: "offer" });
		const named = base?.model;
		setModel(
			named !== undefined && WHISPER_MODELS.includes(named)
				? named
				: DEFAULT_WHISPER_MODEL,
		);
	}, [unavailable]);

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
					}
				},
				() => undefined,
			);
		}, POLL_EVERY_MS);
		return () => clearInterval(timer);
	}, [installing, runtime]);

	async function install(): Promise<void> {
		if (request === null) return;
		setPhase({
			name: "installing",
			done: 0,
			total: sizeKnown ? (unavailable?.downloadBytes ?? 0) : 0,
		});
		try {
			await runtime.client.installTool(request);
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

	if (unavailable === null) return null;
	const command = unavailable.installCommand;
	const canInstall = request !== null && phase.name !== "command_only";
	// The size the service announced is that of its own model.
	const sizeKnown = model === (base?.model ?? DEFAULT_WHISPER_MODEL);
	const percent =
		phase.name === "installing" && phase.total > 0
			? Math.min(100, Math.round((phase.done / phase.total) * 100))
			: 0;

	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{t("ui.editor.auto_captions.install.title")}
					</DialogTitle>
					<DialogDescription>
						{t("ui.editor.auto_captions.install.description")}
					</DialogDescription>
				</DialogHeader>

				<div className="flex flex-col gap-3 text-sm">
					<div>
						<p lang="en">
							<UntrustedText>{unavailable.message}</UntrustedText>
						</p>
						{unavailable.hint !== undefined && (
							<p lang="en" className="text-muted-foreground">
								<UntrustedText>{unavailable.hint}</UntrustedText>
							</p>
						)}
					</div>

					{unavailable.missing.length > 0 && (
						<div>
							<p className="font-medium">
								{t("ui.editor.auto_captions.install.missing")}
							</p>
							<ul className="mt-1 list-disc pl-5 text-xs">
								{unavailable.missing.map((item) => (
									<li key={item.name}>
										<span className="font-mono">
											<UntrustedText>{item.name}</UntrustedText>
										</span>
										{item.bytes !== undefined &&
											` · ${megabytes(item.bytes, locale)} MB`}
										{item.host !== undefined && (
											<>
												{" · "}
												{t("ui.editor.auto_captions.install.source", {
													host: item.host,
												})}
											</>
										)}
									</li>
								))}
							</ul>
						</div>
					)}

					{choosesModel && phase.name === "offer" && (
						<div className="flex flex-col gap-1">
							<Label htmlFor="install-model" className="text-xs">
								{t("ui.editor.auto_captions.install.model")}
							</Label>
							<select
								id="install-model"
								value={model}
								className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
								onChange={(event) => setModel(event.target.value)}
							>
								{WHISPER_MODELS.map((name) => (
									<option key={name} value={name}>
										{name}
									</option>
								))}
							</select>
							<p className="text-xs text-muted-foreground">
								{t("ui.editor.auto_captions.install.model_hint")}
							</p>
						</div>
					)}

					{unavailable.downloadBytes !== undefined && sizeKnown && (
						<p data-testid="install-size">
							{t("ui.editor.auto_captions.install.size", {
								megabytes: megabytes(unavailable.downloadBytes, locale),
							})}
						</p>
					)}

					{phase.name === "installing" && (
						<div>
							<Progress
								value={percent}
								aria-label={t("ui.editor.auto_captions.install.progress")}
								className="h-2"
							/>
							<p className="mt-1 text-xs [font-variant-numeric:tabular-nums]">
								{t("ui.editor.auto_captions.install.installing", {
									done: megabytes(phase.done, locale),
									total: megabytes(phase.total, locale),
								})}
							</p>
						</div>
					)}

					{phase.name === "done" && (
						<p role="status">{t("ui.editor.auto_captions.install.done")}</p>
					)}

					{phase.name === "command_only" && (
						<p role="status" className="text-muted-foreground">
							{t("ui.editor.auto_captions.install.forbidden")}
						</p>
					)}

					{phase.name === "failed" && (
						<div role="alert">
							<p className="flex items-center gap-1.5 font-medium text-destructive">
								<CircleAlert aria-hidden="true" className="size-4" />
								{errorHeadline(t, phase.error.code)}
							</p>
							<p lang="en">
								<UntrustedText>{phase.error.message}</UntrustedText>
							</p>
							{phase.error.hint !== undefined && (
								<p lang="en" className="text-muted-foreground">
									<UntrustedText>{phase.error.hint}</UntrustedText>
								</p>
							)}
							<p className="font-mono text-xs">{phase.error.code}</p>
						</div>
					)}

					{command !== undefined && (
						<div>
							<p className="text-xs">
								{t("ui.editor.auto_captions.install.command")}
							</p>
							<p className="flex items-center gap-1">
								<input
									readOnly
									value={command}
									aria-label={t("ui.editor.auto_captions.install.command")}
									className="h-8 min-w-0 flex-1 rounded-md border bg-transparent px-2 font-mono text-xs"
									onFocus={(event) => event.currentTarget.select()}
								/>
								<Button
									type="button"
									variant="ghost"
									size="icon-sm"
									aria-label={t("ui.editor.auto_captions.install.copy_command")}
									onClick={() => void navigator.clipboard.writeText(command)}
								>
									<Copy aria-hidden="true" />
								</Button>
							</p>
						</div>
					)}
				</div>

				<DialogFooter>
					{phase.name === "failed" && (
						<Button
							type="button"
							variant="outline"
							onClick={() =>
								void navigator.clipboard.writeText(
									JSON.stringify(phase.error, null, 2),
								)
							}
						>
							{t("ui.editor.auto_captions.install.copy_details")}
						</Button>
					)}
					{phase.name === "done" || phase.name === "command_only" ? (
						<Button type="button" onClick={onClose}>
							{t("ui.editor.auto_captions.install.close")}
						</Button>
					) : (
						<>
							<Button type="button" variant="outline" onClick={onClose}>
								{t("ui.editor.auto_captions.install.cancel")}
							</Button>
							{canInstall && (
								<Button
									type="button"
									disabled={phase.name === "installing"}
									onClick={() => void install()}
								>
									{t("ui.editor.auto_captions.install.install")}
								</Button>
							)}
						</>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
