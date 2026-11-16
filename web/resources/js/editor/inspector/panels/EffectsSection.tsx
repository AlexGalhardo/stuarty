import { Trash2 } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	defaultEffect,
	EFFECT_KINDS,
	EFFECT_PARAMS,
	type EffectKind,
	type EffectValue,
	effectsOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const isKind = (value: string): value is EffectKind =>
	(EFFECT_KINDS as readonly string[]).includes(value);

const text = (value: number | undefined): string =>
	value === undefined ? "" : String(Number(value.toFixed(3)));

// The Effects section of a clip (SDD-012 section 12.1): the effects it carries,
// each with the parameters of its kind, and a picker that adds one. Every change
// sends the whole list as one `clip.set_effects` (SDD-002 C.68); the core fills
// the defaults, rounds and checks the ranges, and the preview draws the result.
export default function EffectsSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("clip.set_effects");
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (!exposed || item === undefined) return null;
	const effects: EffectValue[] = [...(item.effects ?? [])];
	const free = EFFECT_KINDS.filter(
		(kind) => !effects.some((effect) => effect.kind === kind),
	);
	const change = (next: EffectValue[]) => send(effectsOperations(itemId, next));

	return (
		<Section title={t("ui.editor.effects.title")} name="effects">
			{effects.map((effect, index) => {
				const name = t(`ui.editor.effects.kind.${effect.kind}`);
				return (
					<div
						key={effect.kind}
						data-effect={effect.kind}
						className="flex flex-col gap-1 rounded border border-input p-2"
					>
						<div className="flex items-center justify-between">
							<p className="text-xs font-medium">{name}</p>
							<Button
								type="button"
								size="icon-sm"
								variant="ghost"
								aria-label={t("ui.editor.effects.remove", { effect: name })}
								disabled={!canEdit}
								onClick={() =>
									void change(effects.filter((_, i) => i !== index))
								}
							>
								<Trash2 aria-hidden="true" />
							</Button>
						</div>
						{EFFECT_PARAMS[effect.kind].map((param) => (
							<NumberField
								key={param.name}
								label={t("ui.editor.effects.field", {
									effect: name,
									param: t(`ui.editor.effects.param.${param.name}`),
								})}
								stored={text(effect[param.name])}
								disabled={!canEdit}
								step={param.step}
								min={param.min}
								max={param.max}
								commit={(value) =>
									change(
										effects.map((e, i) =>
											i === index ? { ...e, [param.name]: value } : e,
										),
									)
								}
							/>
						))}
					</div>
				);
			})}
			{free.length > 0 && (
				<SelectField
					label={t("ui.editor.effects.add")}
					stored=""
					disabled={!canEdit}
					options={[
						{ value: "", label: t("ui.editor.effects.choose") },
						...free.map((kind) => ({
							value: kind,
							label: t(`ui.editor.effects.kind.${kind}`),
						})),
					]}
					commit={async (value) =>
						isKind(value)
							? change([...effects, defaultEffect(value)])
							: { ok: true }
					}
				/>
			)}
		</Section>
	);
}
