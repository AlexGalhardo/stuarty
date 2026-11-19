import type { ReactNode } from "react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale, useT } from "@/lib/i18n";
import {
	type Edge,
	MOTION_DIRECTIONS,
	MOTION_PRESETS,
	type MotionDirection,
	type MotionPreset,
	motionPresetOperations,
	motionTakesAmount,
	motionTakesDirection,
} from "../../commands/builders";
import { localizeDecimalInput, normalizeTimeInput } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import type { FieldError } from "../fields/commitField";
import { FieldMessage } from "../fields/FieldFrame";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const SELECT_CLASS =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30";

const isPreset = (value: string): value is MotionPreset =>
	(MOTION_PRESETS as readonly string[]).includes(value);
const isDirection = (value: string): value is MotionDirection =>
	(MOTION_DIRECTIONS as readonly string[]).includes(value);

// The Motion presets section (RF-082): fade, slide, scale, pop, push in and pull
// out on the start or the end of a visual item, as ordinary keyframes written by
// the core (`animation.apply_preset`), so they are edited afterwards in the
// Keyframes section. The core judges the length, the distance and the factor; an
// empty field takes the default of the preset.
export default function MotionSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const [preset, setPreset] = useState<MotionPreset>("fade");
	const [edge, setEdge] = useState<Edge>("in");
	const [direction, setDirection] = useState<MotionDirection>("left");
	const [length, setLength] = useState("");
	const [amount, setAmount] = useState("");
	// The core's refusal of the last "Apply" (a length longer than the item, an
	// amount out of range), shown as received.
	const [error, setError] = useState<FieldError | null>(null);

	const typed = length.trim();
	const time =
		typed === ""
			? null
			: normalizeTimeInput(localizeDecimalInput(typed, locale));
	const lengthValid = time === null || time.ok;
	const amountText = amount.trim();
	const amountValue =
		amountText === ""
			? undefined
			: Number(localizeDecimalInput(amountText, locale));
	const amountValid = amountValue === undefined || Number.isFinite(amountValue);
	const takesDirection = motionTakesDirection(preset);
	const takesAmount = motionTakesAmount(preset);

	return (
		<Section title={t("ui.editor.motion.title")}>
			<div className="grid grid-cols-[96px_1fr] items-center gap-x-2 gap-y-1">
				<Label htmlFor={`motion-preset-${itemId}`} className="text-xs">
					{t("ui.editor.motion.preset")}
				</Label>
				<select
					id={`motion-preset-${itemId}`}
					value={preset}
					disabled={!canEdit}
					className={SELECT_CLASS}
					onChange={(event) => {
						if (isPreset(event.target.value)) setPreset(event.target.value);
					}}
				>
					{MOTION_PRESETS.map((name) => (
						<option key={name} value={name}>
							{t(`ui.editor.motion.preset_name.${name}`)}
						</option>
					))}
				</select>
				<Label htmlFor={`motion-edge-${itemId}`} className="text-xs">
					{t("ui.editor.animation.edge")}
				</Label>
				<select
					id={`motion-edge-${itemId}`}
					value={edge}
					disabled={!canEdit}
					className={SELECT_CLASS}
					onChange={(event) =>
						setEdge(event.target.value === "out" ? "out" : "in")
					}
				>
					<option value="in">{t("ui.editor.animation.edge_in")}</option>
					<option value="out">{t("ui.editor.animation.edge_out")}</option>
				</select>
				<Label htmlFor={`motion-length-${itemId}`} className="text-xs">
					{t("ui.editor.animation.duration")}
				</Label>
				<Input
					id={`motion-length-${itemId}`}
					value={length}
					placeholder={t("ui.editor.motion.use_default")}
					autoComplete="off"
					spellCheck={false}
					disabled={!canEdit}
					aria-invalid={lengthValid ? undefined : true}
					className="h-8 px-2 font-mono text-sm"
					onChange={(event) => setLength(event.target.value)}
				/>
				{takesDirection && (
					<>
						<Label htmlFor={`motion-direction-${itemId}`} className="text-xs">
							{t("ui.editor.motion.direction")}
						</Label>
						<select
							id={`motion-direction-${itemId}`}
							value={direction}
							disabled={!canEdit}
							className={SELECT_CLASS}
							onChange={(event) => {
								if (isDirection(event.target.value)) {
									setDirection(event.target.value);
								}
							}}
						>
							{MOTION_DIRECTIONS.map((name) => (
								<option key={name} value={name}>
									{t(`ui.editor.motion.direction_name.${name}`)}
								</option>
							))}
						</select>
					</>
				)}
				{takesAmount && (
					<>
						<Label htmlFor={`motion-amount-${itemId}`} className="text-xs">
							{t(`ui.editor.motion.amount_of.${preset}`)}
						</Label>
						<Input
							id={`motion-amount-${itemId}`}
							value={amount}
							placeholder={t("ui.editor.motion.use_default")}
							inputMode="decimal"
							autoComplete="off"
							disabled={!canEdit}
							aria-invalid={amountValid ? undefined : true}
							className="h-8 px-2 font-mono text-sm"
							onChange={(event) => setAmount(event.target.value)}
						/>
					</>
				)}
			</div>
			{!lengthValid && (
				<p role="alert" className="text-xs text-destructive">
					{t("ui.editor.time.hint")}
				</p>
			)}
			<div>
				<Button
					type="button"
					size="sm"
					disabled={!canEdit || !lengthValid || !amountValid}
					onClick={() =>
						void send(
							motionPresetOperations({
								itemId,
								preset,
								edge,
								duration: time?.ok ? time.value : undefined,
								direction: takesDirection ? direction : undefined,
								amount: takesAmount ? amountValue : undefined,
							}),
						).then((result) => setError(result.ok ? null : result.error))
					}
				>
					{t("ui.editor.animation.apply")}
				</Button>
			</div>
			<FieldMessage
				id={`motion-${itemId}-message`}
				state={{ error, remoteActor: null }}
			/>
		</Section>
	);
}
