import { type ReactNode, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import type { Processor } from "@/generated/timeline";
import { useT } from "@/lib/i18n";
import type { AudioLevels, AudioMeasureResult } from "../../api/types";
import {
	applyPresetOperations,
	type ProcessorTarget,
	processorAddOperations,
	processorMoveOperations,
	processorRemoveOperations,
	processorSetOperations,
} from "../../commands/processorBuilders";
import {
	AUDIO_PRESETS,
	AUDIO_PROCESSORS,
	type AudioPresetId,
	MAX_PROCESSORS,
	type ProcessorKind,
	type ProcessorParam,
} from "../../commands/processorTable";
import { displayEnd, displayStart, fpsOf } from "../../commands/units";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import { readTimeDisplay } from "../../state/prefs";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const BAND = /^b(\d)_(hz|gain_db|q)$/;
const UNITLESS = new Set(["ratio", "Q", "fraction"]);

// The label of a parameter: a band of the equaliser says which band, the others have a name of their own.
function useParamLabel(): (name: string) => string {
	const t = useT();
	return (name) => {
		const band = BAND.exec(name);
		if (band !== null)
			return t(`ui.editor.processors.band.${band[2]}`, { n: band[1] ?? "" });
		return t(`ui.editor.processors.param.${name}`);
	};
}

// A step the arrow keys take, from the size of the range (the range is the core's).
function stepOf(spec: ProcessorParam): number {
	const span = spec.max - spec.min;
	if (span >= 200) return 1;
	return span <= 2 ? 0.01 : 0.1;
}

function ProcessorCard({
	target,
	index,
	count,
	processor,
}: {
	target: ProcessorTarget;
	index: number;
	count: number;
	processor: Processor;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const actions = useActions();
	const send = useFieldCommit();
	const paramLabel = useParamLabel();
	const kind = AUDIO_PROCESSORS.find((k) => k.kind === processor.kind);
	if (kind === undefined) return null;
	const name = t(`ui.editor.processors.kind.${processor.kind}`);
	const isItem = "itemId" in target;
	return (
		<li
			className="flex flex-col gap-2 rounded-lg border p-2"
			data-processor-index={index}
			data-processor-kind={processor.kind}
		>
			<div className="flex flex-wrap items-center justify-between gap-1">
				<span className="text-sm font-medium">{name}</span>
				<div className="flex gap-1">
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={!canEdit || index === 0}
						aria-label={`${name} ${index + 1}: ${t("ui.editor.processors.move_up")}`}
						onClick={() =>
							void actions.runOperations(
								processorMoveOperations(target, index, index - 1),
							)
						}
					>
						{t("ui.editor.processors.move_up")}
					</Button>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={!canEdit || index === count - 1}
						aria-label={`${name} ${index + 1}: ${t("ui.editor.processors.move_down")}`}
						onClick={() =>
							void actions.runOperations(
								processorMoveOperations(target, index, index + 1),
							)
						}
					>
						{t("ui.editor.processors.move_down")}
					</Button>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={!canEdit}
						aria-label={`${name} ${index + 1}: ${t("ui.editor.processors.remove")}`}
						onClick={() =>
							void actions.runOperations(
								processorRemoveOperations(target, index),
							)
						}
					>
						{t("ui.editor.processors.remove")}
					</Button>
				</div>
			</div>
			<ToggleField
				label={`${name} ${index + 1}: ${t("ui.editor.processors.enabled")}`}
				stored={processor.disabled !== true}
				disabled={!canEdit}
				commit={(enabled) =>
					send(processorSetOperations(target, index, { enabled }))
				}
			/>
			<details>
				<summary className="cursor-pointer text-xs text-muted-foreground">
					{t("ui.editor.processors.params")}
				</summary>
				<div className="mt-2 flex flex-col gap-2">
					{kind.params
						.filter(
							(spec) =>
								isItem ||
								(spec.name !== "profile_start_ms" &&
									spec.name !== "profile_end_ms"),
						)
						.map((spec) => (
							<NumberField
								key={spec.name}
								label={`${name} ${index + 1}: ${paramLabel(spec.name)}`}
								stored={String(processor.params?.[spec.name] ?? spec.default)}
								disabled={!canEdit}
								step={stepOf(spec)}
								min={spec.min}
								max={spec.max}
								suffix={UNITLESS.has(spec.unit) ? undefined : spec.unit}
								commit={(value) =>
									send(
										processorSetOperations(target, index, {
											params: { [spec.name]: value },
										}),
									)
								}
							/>
						))}
				</div>
			</details>
		</li>
	);
}

function Level({ value, unit }: { value: number | null; unit: string }) {
	const t = useT();
	return value === null
		? t("ui.editor.processors.no_level")
		: `${value.toFixed(1)} ${unit}`;
}

function LevelsRow({
	label,
	levels,
}: {
	label: string;
	levels: AudioLevels;
}): ReactNode {
	return (
		<tr>
			<th scope="row" className="pr-2 text-left font-normal">
				{label}
			</th>
			<td className="pr-2 font-mono tabular-nums">
				<Level value={levels.integrated_lufs} unit="LUFS" />
			</td>
			<td className="pr-2 font-mono tabular-nums">
				<Level value={levels.true_peak_dbtp} unit="dBTP" />
			</td>
			<td className="pr-2 font-mono tabular-nums">
				<Level value={levels.noise_floor_dbfs} unit="dBFS" />
			</td>
			<td className="font-mono tabular-nums">
				<Level value={levels.speech_to_noise_db} unit="dB" />
			</td>
		</tr>
	);
}

// The before and after meters: the core measures the sound with and without the chain (AudioMeasure).
function Meters({ target }: { target: ProcessorTarget }): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const projectId = useDocumentState((s) => s.projectId);
	const [state, setState] = useState<
		| { name: "idle" }
		| { name: "busy" }
		| { name: "failed" }
		| { name: "done"; result: AudioMeasureResult }
	>({ name: "idle" });
	const measure = async (): Promise<void> => {
		setState({ name: "busy" });
		try {
			const result = await runtime.client.measureAudio(
				projectId,
				"itemId" in target ? { itemId: target.itemId } : {},
			);
			setState({ name: "done", result });
		} catch {
			setState({ name: "failed" });
		}
	};
	return (
		<div className="flex flex-col gap-2" data-meters="">
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={state.name === "busy"}
				onClick={() => void measure()}
			>
				{state.name === "busy"
					? t("ui.editor.processors.measuring")
					: t("ui.editor.processors.measure")}
			</Button>
			{state.name === "failed" && (
				<p role="alert" className="text-xs text-destructive">
					{t("ui.editor.processors.measure_failed")}
				</p>
			)}
			{state.name === "done" && (
				<table className="text-xs" aria-live="polite">
					<thead>
						<tr>
							<td />
							<th scope="col" className="pr-2 text-left font-normal">
								{t("ui.editor.processors.loudness")}
							</th>
							<th scope="col" className="pr-2 text-left font-normal">
								{t("ui.editor.processors.true_peak")}
							</th>
							<th scope="col" className="pr-2 text-left font-normal">
								{t("ui.editor.processors.noise_floor")}
							</th>
							<th scope="col" className="text-left font-normal">
								{t("ui.editor.processors.speech_to_noise")}
							</th>
						</tr>
					</thead>
					<tbody>
						<LevelsRow
							label={t("ui.editor.processors.before")}
							levels={state.result.before}
						/>
						<LevelsRow
							label={t("ui.editor.processors.after")}
							levels={state.result.after}
						/>
					</tbody>
				</table>
			)}
		</div>
	);
}

const POLL_MS = 800;
const POLL_TRIES = 150;

// The processed sound: an audio-only render of the revision (format wav) of the item, made by the mixer of the export,
// then played from the file the job wrote (SDD-004 section 7.1).
function Listen({ target }: { target: ProcessorTarget }): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const projectId = useDocumentState((s) => s.projectId);
	const fps = fpsOf(useDocumentState((s) => s.document?.settings.fps));
	const item = useDocumentState((s) =>
		"itemId" in target ? s.document?.items[target.itemId] : undefined,
	);
	const [phase, setPhase] = useState<
		| { name: "idle" }
		| { name: "rendering"; jobId: string }
		| { name: "failed" }
		| { name: "ready"; url: string }
	>({ name: "idle" });
	const jobId = phase.name === "rendering" ? phase.jobId : null;
	useEffect(() => {
		if (jobId === null) return;
		let alive = true;
		void (async () => {
			for (let tries = 0; tries < POLL_TRIES && alive; tries++) {
				await new Promise((resolve) => setTimeout(resolve, POLL_MS));
				const job = await runtime.client.getJob(jobId).catch(() => null);
				if (!alive || job === null) continue;
				if (job.state === "succeeded") {
					const url = job.result?.export?.download_url;
					setPhase(
						url === undefined ? { name: "failed" } : { name: "ready", url },
					);
					return;
				}
				if (job.state === "failed" || job.state === "cancelled") {
					setPhase({ name: "failed" });
					return;
				}
			}
			if (alive) setPhase({ name: "failed" });
		})();
		return () => {
			alive = false;
		};
	}, [jobId, runtime]);
	const render = async (): Promise<void> => {
		try {
			const options =
				item === undefined
					? { format: "wav" }
					: {
							format: "wav",
							start: displayStart(item, fps, readTimeDisplay()),
							end: displayEnd(item, fps, readTimeDisplay()),
						};
			const job = await runtime.client.startExport(projectId, options);
			setPhase({ name: "rendering", jobId: job.job_id });
		} catch {
			setPhase({ name: "failed" });
		}
	};
	return (
		<div className="flex flex-col gap-2">
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={phase.name === "rendering"}
				onClick={() => void render()}
			>
				{phase.name === "rendering"
					? t("ui.editor.processors.rendering")
					: t("ui.editor.processors.listen")}
			</Button>
			{phase.name === "failed" && (
				<p role="alert" className="text-xs text-destructive">
					{t("ui.editor.processors.listen_failed")}
				</p>
			)}
			{phase.name === "ready" && (
				// biome-ignore lint/a11y/useMediaCaption: it is a render of the sound of the item, with no speech track to caption.
				<audio
					controls
					src={phase.url}
					aria-label={t("ui.editor.processors.player")}
				/>
			)}
		</div>
	);
}

// The Audio processors section of a clip, an audio item or a track (SDD-004 section 7.1): the chain in the user's
// order with the controls of each processor, the presets, the meters before and after, and a way to listen.
export default function AudioProcessors({
	target,
}: {
	target: ProcessorTarget;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const actions = useActions();
	const exposed = useCommandsExposed("audio.processor_add");
	const chain = useDocumentState((s) =>
		"itemId" in target
			? s.document?.items[target.itemId]?.processors
			: s.document?.tracks[target.trackId]?.processors,
	);
	const [kind, setKind] = useState<ProcessorKind>("eq");
	const [preset, setPreset] = useState<AudioPresetId>("voice");
	if (!exposed) return null;
	const list = chain ?? [];
	return (
		<Section title={t("ui.editor.processors.title")} name="audio-processors">
			<div className="flex flex-col gap-2">
				<SelectField
					label={t("ui.editor.processors.preset_label")}
					stored={preset}
					disabled={!canEdit}
					options={AUDIO_PRESETS.map((id) => ({
						value: id,
						label: t(`ui.editor.processors.presets.${id}`),
					}))}
					commit={async (value) => {
						setPreset(value as AudioPresetId);
						return { ok: true };
					}}
				/>
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={!canEdit}
					onClick={() =>
						void actions.runOperations(applyPresetOperations(target, preset))
					}
				>
					{t("ui.editor.processors.preset_apply")}
				</Button>
				{list.length === 0 ? (
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.processors.empty")}
					</p>
				) : (
					<ol className="flex flex-col gap-2">
						{list.map((processor, index) => (
							<ProcessorCard
								// biome-ignore lint/suspicious/noArrayIndexKey: the chain has no ids; its order is its identity.
								key={index}
								target={target}
								index={index}
								count={list.length}
								processor={processor}
							/>
						))}
					</ol>
				)}
				<SelectField
					label={t("ui.editor.processors.add_kind")}
					stored={kind}
					disabled={!canEdit}
					options={AUDIO_PROCESSORS.map((spec) => ({
						value: spec.kind,
						label: t(`ui.editor.processors.kind.${spec.kind}`),
					}))}
					commit={async (value) => {
						setKind(value as ProcessorKind);
						return { ok: true };
					}}
				/>
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={!canEdit || list.length >= MAX_PROCESSORS}
					onClick={() =>
						void actions.runOperations(processorAddOperations(target, kind))
					}
				>
					{t("ui.editor.processors.add")}
				</Button>
				<Meters target={target} />
				<Listen target={target} />
			</div>
		</Section>
	);
}
