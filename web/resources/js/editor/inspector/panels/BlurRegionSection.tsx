import { EyeOff, Info, Plus, ShieldAlert } from "lucide-react";
import { type ReactNode, useState } from "react";

import { Segment } from "@/components/Segment";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { BLUR_STRENGTHS } from "../../commands/blurStrengths";
import { blurRegionOperations } from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import NumberField from "../fields/NumberField";
import TimeCommitField from "../fields/TimeCommitField";
import Section, { ToolLine } from "../Section";

// What a region starts with: a strip across the lower third, as wide as an e-mail line.
const START_REGION = { x: 0.1, y: 0.62, width: 0.8, height: 0.08 };

// The screen "Blur sensitive data" (final design 29, SDD-012 section 12.1, SDD-002 C.90): the region as a 2 by 2 grid
// of X, Y, W and H (shares of the picture), the time, and the strength, a choice between the named strengths of the
// core; one button sends `clip.blur_region`. The core makes the blurred copy on a Blur track, turns the strength into
// a radius for the project, holds the minimum radius, and says when the region is close to an edge of the picture.
export default function BlurRegionSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const exposed = useCommandsExposed("clip.blur_region");
	const [region, setRegion] = useState(START_REGION);
	const [start, setStart] = useState<string | undefined>(undefined);
	const [duration, setDuration] = useState<string | undefined>(undefined);
	const [strength, setStrength] = useState<string | undefined>(undefined);
	const [busy, setBusy] = useState(false);
	if (!exposed) return null;
	const chosen = strength ?? BLUR_STRENGTHS[0]?.id;
	const share = (label: string, key: keyof typeof START_REGION): ReactNode => (
		<NumberField
			label={t(label)}
			stored={String(region[key])}
			disabled={!canEdit}
			step={0.05}
			min={0}
			max={1}
			percentInput
			suffix={t("ui.editor.clip.mask_unit")}
			commit={async (value) => {
				setRegion((was) => ({ ...was, [key]: value }));
				return { ok: true };
			}}
		/>
	);
	return (
		<div
			data-section="blur-region"
			className="flex flex-col gap-3"
			data-testid="blur-region"
		>
			<h3 className="flex items-center gap-2 text-sm font-semibold">
				<EyeOff
					aria-hidden="true"
					className="size-4 shrink-0 text-muted-foreground"
				/>
				{t("ui.editor.blur_region.title")}
			</h3>
			<Section title={t("ui.editor.blur_region.group_region")}>
				<ToolLine name="clip.blur_region" />
				<div className="grid grid-cols-2 gap-x-3 gap-y-2" data-grid="region">
					{share("ui.editor.blur_region.x", "x")}
					{share("ui.editor.blur_region.y", "y")}
					{share("ui.editor.blur_region.width", "width")}
					{share("ui.editor.blur_region.height", "height")}
				</div>
			</Section>
			<Section title={t("ui.editor.blur_region.group_time")}>
				<TimeCommitField
					label={t("ui.editor.blur_region.start")}
					stored={start ?? ""}
					disabled={!canEdit}
					commit={async (value) => {
						setStart(value);
						return { ok: true };
					}}
				/>
				<TimeCommitField
					label={t("ui.editor.blur_region.duration")}
					stored={duration ?? ""}
					disabled={!canEdit}
					commit={async (value) => {
						setDuration(value);
						return { ok: true };
					}}
				/>
			</Section>
			<Section title={t("ui.editor.blur_region.group_strength")}>
				{BLUR_STRENGTHS.length > 0 && chosen !== undefined && (
					<Segment
						label={t("ui.editor.blur_region.group_strength")}
						value={chosen}
						onChange={setStrength}
						options={BLUR_STRENGTHS.map((entry) => ({
							value: entry.id,
							label: t(`ui.editor.blur_region.strengths.${entry.id}`),
						}))}
						className="w-full *:flex-1"
					/>
				)}
				<p className="flex items-start gap-2 text-xs text-muted-foreground">
					<ShieldAlert
						aria-hidden="true"
						className="mt-0.5 size-3.5 shrink-0"
					/>
					{t("ui.editor.blur_region.strength_hint")}
				</p>
			</Section>
			<Button
				type="button"
				disabled={!canEdit || busy}
				className="w-full"
				onClick={async () => {
					setBusy(true);
					try {
						await actions.runOperations(
							blurRegionOperations(itemId, {
								...region,
								start,
								duration,
								strength,
							}),
						);
					} finally {
						setBusy(false);
					}
				}}
			>
				<Plus aria-hidden="true" />
				{t("ui.editor.blur_region.apply")}
			</Button>
			<p className="flex items-start gap-2 text-xs text-muted-foreground">
				<Info aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
				{t("ui.editor.blur_region.hint")}
			</p>
		</div>
	);
}
