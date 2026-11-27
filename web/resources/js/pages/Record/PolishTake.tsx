import { type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { FieldLabel } from "@/components/FieldHelp";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { ApiClient } from "../../editor/api/client";
import type { Job, ProjectGetResult, Recording } from "../../editor/api/types";
import { exportFileUrl, proxyUrl } from "../../editor/api/urls";
import {
	autoZoomOperations,
	backgroundOperations,
	CORNERS,
	type Corner,
	clickHighlightOperations,
	cursorOperations,
	LAYOUTS,
	type Layout,
	layoutOperations,
	type Operations,
	trimEndOperations,
	zoomClearOperations,
} from "../../editor/commands/builders";
import { PromoteProjectForm } from "../../editor/layout/PromoteProjectDialog";

export interface TakeStyle {
	layout: Layout;
	corner: Corner;
	background: string;
}

const COLORS = ["#111827", "#233459", "#F1F5F9"];

export function StyleControls({
	value,
	onChange,
}: {
	value: TakeStyle;
	onChange(value: TakeStyle): void;
}): ReactNode {
	const t = useT();
	const field = "rounded-md border border-border bg-background px-2 py-1.5";
	return (
		<fieldset className="grid gap-3">
			<legend className="mb-3 font-semibold">{t("ui.record.style")}</legend>
			<label className="grid gap-1 text-sm">
				{t("ui.record.layout")}
				<select
					className={field}
					value={value.layout}
					onChange={(event) => {
						const next = LAYOUTS.find(
							(layout) => layout === event.target.value,
						);
						if (next !== undefined) onChange({ ...value, layout: next });
					}}
				>
					{LAYOUTS.map((layout) => (
						<option key={layout} value={layout}>
							{t(`ui.record.layouts.${layout}`)}
						</option>
					))}
				</select>
			</label>
			{(value.layout === "pip" || value.layout === "camera_focus") && (
				<label className="grid gap-1 text-sm">
					{t("ui.record.camera_position")}
					<select
						className={field}
						value={value.corner}
						onChange={(event) => {
							const next = CORNERS.find(
								(corner) => corner === event.target.value,
							);
							if (next !== undefined) onChange({ ...value, corner: next });
						}}
					>
						{CORNERS.map((corner) => (
							<option key={corner} value={corner}>
								{t(`ui.record.corners.${corner}`)}
							</option>
						))}
					</select>
				</label>
			)}
			<label className="grid gap-1 text-sm">
				{t("ui.record.background")}
				<select
					className={field}
					value={value.background}
					onChange={(event) =>
						onChange({ ...value, background: event.target.value })
					}
				>
					<option value="">{t("ui.record.no_background")}</option>
					{COLORS.map((color) => (
						<option key={color} value={color}>
							{color}
						</option>
					))}
				</select>
			</label>
		</fieldset>
	);
}

function clipFor(
	project: ProjectGetResult | null,
	assetId: string | undefined,
) {
	if (assetId === undefined) return undefined;
	return Object.values(project?.document?.items ?? {}).find(
		(item) => item.type === "clip" && item.asset === assetId,
	);
}

export function PolishTake({
	client,
	take,
	style,
	setStyle,
	onReset,
}: {
	client: ApiClient;
	take: Recording;
	style: TakeStyle;
	setStyle(value: TakeStyle): void;
	onReset(): void;
}): ReactNode {
	const t = useT();
	const [project, setProject] = useState<ProjectGetResult | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const [job, setJob] = useState<Job | null>(null);
	const [trim, setTrim] = useState("");
	const [promote, setPromote] = useState(false);
	const [promoted, setPromoted] = useState<string | null>(null);
	const projectId = take.project_id;
	const screen = clipFor(project, take.asset_id);
	const camera = clipFor(
		project,
		take.tracks?.find((track) => track.kind === "camera")?.asset_id,
	);

	useEffect(() => {
		let active = true;
		void client.getProject(projectId).then(
			(result) => {
				if (active) setProject(result);
			},
			(error) => {
				if (active) setFailure(failureOf(error));
			},
		);
		return () => {
			active = false;
		};
	}, [client, projectId]);

	const exporting = job?.state === "queued" || job?.state === "running";
	const jobId = job?.job_id;
	useEffect(() => {
		if (!exporting || jobId === undefined) return;
		let active = true;
		const timer = window.setInterval(() => {
			void client.getJob(jobId).then(
				(result) => {
					if (active) setJob(result);
				},
				(error) => {
					if (active) setFailure(failureOf(error));
				},
			);
		}, 1000);
		return () => {
			active = false;
			window.clearInterval(timer);
		};
	}, [client, exporting, jobId]);

	async function run(action: () => Promise<void>): Promise<void> {
		setBusy(true);
		setFailure(null);
		try {
			await action();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	function edit(operations: Operations): void {
		void run(async () => {
			const latest = await client.getProject(projectId);
			await client.edit(projectId, {
				base_revision: latest.revision,
				idempotency_key: crypto.randomUUID(),
				operations,
			});
			setProject(await client.getProject(projectId));
		});
	}

	const canPolish = screen !== undefined && !busy;
	return (
		<div className="grid min-w-0 gap-4" data-testid="record-polish">
			<h2 className="text-lg font-semibold">{t("ui.record.polish")}</h2>
			{take.asset_id !== undefined && (
				// The captured video has no caption track until the creator adds captions in the editor.
				// biome-ignore lint/a11y/useMediaCaption: user-recorded source preview
				<video
					controls
					preload="metadata"
					className="aspect-video w-full rounded-md bg-black"
					aria-label={t("ui.record.preview")}
					src={proxyUrl(projectId, take.asset_id)}
				/>
			)}
			{screen === undefined && (
				<Button
					type="button"
					disabled={busy || project === null}
					onClick={() =>
						void run(async () => {
							await client.placeRecording(take.recording_id);
							setProject(await client.getProject(projectId));
						})
					}
				>
					{t("ui.editor.record.place")}
				</Button>
			)}
			{screen !== undefined && (
				<>
					<StyleControls value={style} onChange={setStyle} />
					<Button
						type="button"
						variant="outline"
						disabled={
							!canPolish ||
							(style.layout !== "screen_only" && camera === undefined)
						}
						onClick={() =>
							edit([
								...layoutOperations(screen.id, {
									layout: style.layout,
									cameraItemId: camera?.id,
									corner: style.corner,
								}),
								...backgroundOperations(
									screen.id,
									style.background === ""
										? { kind: "none" }
										: { kind: "color", color: style.background },
								),
							])
						}
					>
						{t("ui.record.apply_style")}
					</Button>
					<fieldset
						className="flex flex-wrap gap-2"
						disabled={!canPolish || !take.events_count}
					>
						<legend className="mb-2 text-sm font-medium">
							{t("ui.record.pointer")}
						</legend>
						<Button
							type="button"
							variant="outline"
							onClick={() => edit(autoZoomOperations(screen.id, {}))}
						>
							{t("ui.record.zoom")}
						</Button>
						<Button
							type="button"
							variant="outline"
							onClick={() =>
								edit(cursorOperations(screen.id, { visible: true }))
							}
						>
							{t("ui.record.cursor")}
						</Button>
						<Button
							type="button"
							variant="outline"
							onClick={() =>
								edit(clickHighlightOperations(screen.id, { style: "ring" }))
							}
						>
							{t("ui.record.clicks")}
						</Button>
						<Button
							type="button"
							variant="ghost"
							onClick={() =>
								edit([
									...zoomClearOperations(screen.id),
									...cursorOperations(screen.id, { visible: false }),
									...clickHighlightOperations(screen.id, { style: "none" }),
								])
							}
						>
							{t("ui.record.clear_pointer")}
						</Button>
					</fieldset>
					<form
						className="flex flex-wrap items-end gap-2"
						onSubmit={(event) => {
							event.preventDefault();
							edit(trimEndOperations(screen.id, trim.trim()));
						}}
					>
						<div className="grid gap-1 text-sm">
							<FieldLabel
								htmlFor="record-trim-end"
								label={t("ui.record.trim_end")}
								help={t("ui.record.trim_help")}
							/>
							<input
								id="record-trim-end"
								value={trim}
								onChange={(event) => setTrim(event.target.value)}
								placeholder={t("ui.record.trim_example")}
								className="rounded-md border border-border bg-background px-2 py-1.5"
							/>
						</div>
						<Button
							type="submit"
							variant="outline"
							disabled={!canPolish || trim.trim() === ""}
						>
							{t("ui.record.trim")}
						</Button>
					</form>
					<div className="flex flex-wrap gap-2">
						<Button asChild>
							<a href={`/editor/${encodeURIComponent(projectId)}`}>
								{t("ui.record.open_editor")}
							</a>
						</Button>
						<Button
							type="button"
							variant="outline"
							disabled={busy || exporting}
							onClick={() =>
								void run(async () => {
									setJob(
										await client.startExport(projectId, { format: "mp4" }),
									);
								})
							}
						>
							{t("ui.record.export")}
						</Button>
						{promoted === null && (
							<Button
								type="button"
								variant="outline"
								disabled={busy}
								onClick={() => setPromote(true)}
							>
								{t("ui.editor.promote.button")}
							</Button>
						)}
						{promoted !== null && (
							<Button asChild variant="outline">
								<a href={`/projects/${encodeURIComponent(promoted)}`}>
									{t("ui.record.open_project")}
								</a>
							</Button>
						)}
					</div>
				</>
			)}
			{job?.state === "succeeded" && (
				<Button asChild variant="outline">
					<a download href={exportFileUrl(job.job_id)}>
						{t("ui.editor.export.download")}
					</a>
				</Button>
			)}
			{exporting && (
				<p role="status" className="text-sm text-text-muted">
					{t("ui.record.exporting")}
				</p>
			)}
			{(job?.state === "failed" || job?.state === "cancelled") && (
				<p role="alert">{t("ui.record.export_failed")}</p>
			)}
			<FailureNote failure={failure} />
			<Button
				type="button"
				variant="ghost"
				className="justify-self-start"
				disabled={busy}
				onClick={onReset}
			>
				{t("ui.editor.record.again")}
			</Button>
			{promote && (
				<PromoteProjectForm
					open
					initialName={project?.name ?? t("ui.record.title")}
					onOpenChange={setPromote}
					promote={(input) => client.promoteStandaloneVideo(projectId, input)}
					onPromoted={setPromoted}
				/>
			)}
		</div>
	);
}
