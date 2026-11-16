import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import {
	FRAME_RATES,
	projectRenameOperations,
	reframeOperations,
	type SettingsChanges,
	SIZE_PRESETS,
	settingsSetOperations,
} from "../commands/builders";
import { useActions, useCanEdit } from "../commands/useActions";
import { useDocumentState } from "../state/EditorContext";
import { selectFps } from "../state/selectors";
import FpsConfirmDialog, {
	type AdjustedLine,
	type FpsStage,
} from "../ui/FpsConfirmDialog";
import ColorField from "./fields/ColorField";
import { FieldRow } from "./fields/FieldFrame";
import NumberField from "./fields/NumberField";
import SelectField from "./fields/SelectField";
import TextField from "./fields/TextField";
import FontsSection from "./panels/FontsSection";
import MarkersSection from "./panels/MarkersSection";
import TemplateInfoSection from "./panels/TemplateInfoSection";
import { useFieldCommit } from "./useFieldCommit";

const CUSTOM = "custom";
const NO_STATE = { error: null, remoteActor: null } as const;

function rateKey(rate: { num?: number; den?: number } | undefined): string {
	if (rate?.num === undefined) return "30";
	return rate.den === undefined || rate.den === 1
		? String(rate.num)
		: `${rate.num}/${rate.den}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null;
}

// Nothing selected: the project panel (SDD-012 section 12.1): name, size,
// background, loudness target and frame rate. The frame rate asks first and
// then lists what the core had to adjust.
export default function ProjectPanel(): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const name = useDocumentState((s) => s.document?.name ?? "");
	const settings = useDocumentState((s) => s.document?.settings);
	const fps = useDocumentState(selectFps);
	const items = useDocumentState((s) => s.document?.items);
	const [pending, setPending] = useState<string | null>(null);
	const [stage, setStage] = useState<FpsStage>("confirm");
	const [adjusted, setAdjusted] = useState<AdjustedLine[]>([]);
	if (settings === undefined) return null;

	const width = settings.width ?? 1080;
	const height = settings.height ?? 1920;
	const preset =
		SIZE_PRESETS.find((p) => p.width === width && p.height === height)?.id ??
		CUSTOM;
	const current = rateKey(fps);
	const change = (changes: SettingsChanges) =>
		send(settingsSetOperations(changes));

	const labelOf = (id: string): string => {
		const item = items?.[id];
		return item === undefined
			? id
			: (item.name ?? t(`ui.editor.item.${item.type}`));
	};

	const confirm = async (): Promise<void> => {
		if (pending === null) return;
		setStage("busy");
		const outcome = await actions.setFps(pending);
		if (outcome === null || !outcome.ok) {
			// The error is already a notice.
			close();
			return;
		}
		const raw = outcome.result.effective.find(
			(e) => e.op === "settings.set_fps",
		)?.effective.adjusted;
		setAdjusted(
			Array.isArray(raw)
				? raw.flatMap((entry) =>
						isRecord(entry) && typeof entry.id === "string"
							? [
									{
										id: entry.id,
										label: labelOf(entry.id),
										field: String(entry.field ?? ""),
										reason: String(entry.reason ?? ""),
										rescaled: String(entry.rescaled ?? ""),
										final: String(entry.final ?? ""),
									},
								]
							: [],
					)
				: [],
		);
		setStage("done");
	};

	const close = (): void => {
		setPending(null);
		setStage("confirm");
		setAdjusted([]);
	};

	return (
		<div className="flex flex-col gap-3">
			<TextField
				label={t("ui.editor.inspector.name")}
				stored={name}
				disabled={!canEdit}
				maxLength={120}
				commit={(next) => send(projectRenameOperations(next))}
			/>
			<SelectField
				label={t("ui.editor.project.aspect")}
				stored={preset}
				disabled={!canEdit}
				options={[
					...SIZE_PRESETS.map((p) => ({
						value: p.id,
						label: `${p.id} (${p.width} × ${p.height})`,
					})),
					...(preset === CUSTOM
						? [{ value: CUSTOM, label: t("ui.editor.project.custom") }]
						: []),
				]}
				commit={async (value) => {
					const chosen = SIZE_PRESETS.find((p) => p.id === value);
					if (chosen === undefined) return { ok: true };
					return send(reframeOperations(chosen.id));
				}}
			/>
			<NumberField
				label={t("ui.editor.project.width")}
				stored={String(width)}
				disabled={!canEdit}
				step={2}
				min={16}
				max={4096}
				suffix="px"
				commit={(value) => change({ width: value })}
			/>
			<NumberField
				label={t("ui.editor.project.height")}
				stored={String(height)}
				disabled={!canEdit}
				step={2}
				min={16}
				max={4096}
				suffix="px"
				commit={(value) => change({ height: value })}
			/>
			<ColorField
				label={t("ui.editor.project.background")}
				stored={settings.background ?? "#000000"}
				disabled={!canEdit}
				commit={(value) => change({ background: value })}
			/>
			<NumberField
				label={t("ui.editor.project.loudness")}
				stored={String(settings.loudness_target ?? -14)}
				disabled={!canEdit}
				step={1}
				min={-30}
				max={-5}
				suffix="LUFS"
				commit={(value) => change({ loudness_target: value })}
			/>
			<FieldRow label={t("ui.editor.project.fps")} state={NO_STATE}>
				{(ids) => (
					<select
						id={ids.control}
						value={current}
						disabled={!canEdit}
						className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
						onChange={(event) => {
							if (event.target.value !== current) {
								setStage("confirm");
								setPending(event.target.value);
							}
						}}
					>
						{[...new Set([current, ...FRAME_RATES])].map((rate) => (
							<option key={rate} value={rate}>
								{rate}
							</option>
						))}
					</select>
				)}
			</FieldRow>
			<FontsSection />
			<MarkersSection />
			<TemplateInfoSection />
			<FpsConfirmDialog
				open={pending !== null}
				rate={pending ?? ""}
				stage={stage}
				adjusted={adjusted}
				onConfirm={() => void confirm()}
				onClose={close}
			/>
		</div>
	);
}
