import type { ReactNode } from "react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale, useT } from "@/lib/i18n";
import {
	ANIMATION_PRESETS,
	type AnimationPreset,
	type Edge,
	presetOperations,
} from "../../commands/builders";
import { localizeDecimalInput, normalizeTimeInput } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// The core default of a preset length, shown as the hint of an empty field.
const EXAMPLE_LENGTH = "0.4s";

const SELECT_CLASS =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30";

const isPreset = (value: string): value is AnimationPreset =>
	(ANIMATION_PRESETS as readonly string[]).includes(value);

// The Animation section of SDD-012 section 12.1 for any visual item: a preset
// on an edge, with a length when one is typed (`text.apply_preset`). The preset
// is expanded by the core into keyframes; nothing here previews it.
export default function AnimationSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const [preset, setPreset] = useState<AnimationPreset>("fade_up");
	const [edge, setEdge] = useState<Edge>("in");
	const [length, setLength] = useState("");
	const typed = length.trim();
	const time =
		typed === ""
			? null
			: normalizeTimeInput(localizeDecimalInput(typed, locale));
	const valid = time === null || time.ok;

	return (
		<Section title={t("ui.editor.animation.title")}>
			<div className="grid grid-cols-[96px_1fr] items-center gap-x-2 gap-y-1">
				<Label htmlFor={`anim-preset-${itemId}`} className="text-xs">
					{t("ui.editor.animation.preset")}
				</Label>
				<select
					id={`anim-preset-${itemId}`}
					value={preset}
					disabled={!canEdit}
					className={SELECT_CLASS}
					onChange={(event) => {
						if (isPreset(event.target.value)) setPreset(event.target.value);
					}}
				>
					{ANIMATION_PRESETS.map((name) => (
						<option key={name} value={name}>
							{t(`ui.editor.animation.preset_name.${name}`)}
						</option>
					))}
				</select>
				<Label htmlFor={`anim-edge-${itemId}`} className="text-xs">
					{t("ui.editor.animation.edge")}
				</Label>
				<select
					id={`anim-edge-${itemId}`}
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
				<Label htmlFor={`anim-length-${itemId}`} className="text-xs">
					{t("ui.editor.animation.duration")}
				</Label>
				<Input
					id={`anim-length-${itemId}`}
					value={length}
					placeholder={EXAMPLE_LENGTH}
					autoComplete="off"
					spellCheck={false}
					disabled={!canEdit}
					aria-invalid={valid ? undefined : true}
					className="h-8 px-2 font-mono text-sm"
					onChange={(event) => setLength(event.target.value)}
				/>
			</div>
			{!valid && (
				<p role="alert" className="text-xs text-destructive">
					{t("ui.editor.time.hint")}
				</p>
			)}
			<div>
				<Button
					type="button"
					size="sm"
					disabled={!canEdit || !valid}
					onClick={() =>
						void send(
							presetOperations({
								itemId,
								edge,
								preset,
								duration: time?.ok ? time.value : undefined,
							}),
						)
					}
				>
					{t("ui.editor.animation.apply")}
				</Button>
			</div>
		</Section>
	);
}
