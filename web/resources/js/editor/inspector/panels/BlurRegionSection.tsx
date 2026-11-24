import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { blurRegionOperations } from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import NumberField from "../fields/NumberField";
import TimeCommitField from "../fields/TimeCommitField";
import Section from "../Section";

// What a region starts with: a strip across the lower third, as wide as an e-mail line.
const START_REGION = { x: 0.1, y: 0.62, width: 0.8, height: 0.08 };

// The Blur a region section of a clip (SDD-012 section 12.1, SDD-002 C.90): the person gives the region as a
// share of the picture, and optionally the time and the radius; one button sends `clip.blur_region`. The core makes
// the blurred copy on a Blur track, holds the minimum radius, and says when the region is close to an edge of the
// picture, where part of the sharp picture would show through.
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
	const [radius, setRadius] = useState<number | undefined>(undefined);
	const [busy, setBusy] = useState(false);
	const [nearEdge, setNearEdge] = useState(false);
	if (!exposed) return null;
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
		<Section title={t("ui.editor.blur_region.title")} name="blur-region">
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.blur_region.hint")}
			</p>
			{share("ui.editor.blur_region.x", "x")}
			{share("ui.editor.blur_region.y", "y")}
			{share("ui.editor.blur_region.width", "width")}
			{share("ui.editor.blur_region.height", "height")}
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
			<NumberField
				label={t("ui.editor.blur_region.radius")}
				stored={radius === undefined ? "" : String(radius)}
				disabled={!canEdit}
				step={1}
				min={0}
				max={100}
				suffix="px"
				placeholder={t("ui.editor.blur_region.radius_default")}
				commit={async (value) => {
					setRadius(value);
					return { ok: true };
				}}
			/>
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={!canEdit || busy}
				onClick={async () => {
					setBusy(true);
					setNearEdge(false);
					try {
						const outcome = await actions.runOperations(
							blurRegionOperations(itemId, {
								...region,
								start,
								duration,
								radius,
							}),
						);
						if (outcome?.ok === true) {
							const first = outcome.result.effective[0]?.effective;
							setNearEdge(first?.near_edge === true);
						}
					} finally {
						setBusy(false);
					}
				}}
			>
				{t("ui.editor.blur_region.apply")}
			</Button>
			{nearEdge && (
				<p role="status" className="text-xs text-muted-foreground">
					{t("ui.editor.blur_region.near_edge")}
				</p>
			)}
		</Section>
	);
}
