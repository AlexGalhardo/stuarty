import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import {
	type SlotMark,
	slotMarkOperations,
	slotUnmarkOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import SelectField from "../fields/SelectField";
import TextField from "../fields/TextField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const POLICIES = ["fit", "stretch_timeline"] as const;

// The Template section of SDD-012 section 12.1 for a clip, audio, text or
// caption item: the "Template slot" toggle with name, description, required
// and the duration policy of a media slot. Every change is one `slot.mark`
// carrying the whole slot (re-marking with the same name updates it), the
// toggle off is `slot.unmark`. The name stays fixed while the item is a slot,
// because the core refuses another name on an item that already has one.
export default function TemplateSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	// The toggle is on before there is a name to send.
	const [armed, setArmed] = useState(false);
	if (item === undefined) return null;
	const slot = item.slot;
	const media = item.type === "clip" || item.type === "audio";
	const on = slot !== undefined || armed;

	const mark = (changes: Partial<SlotMark>) =>
		send(
			slotMarkOperations(itemId, {
				name: slot?.name ?? changes.name ?? "",
				description: slot?.description ?? "",
				required: slot?.required ?? true,
				...(media ? { durationPolicy: slot?.duration_policy ?? "fit" } : {}),
				...changes,
			}),
		);

	return (
		<Section title={t("ui.editor.template.title")}>
			<ToggleField
				label={t("ui.editor.template.slot")}
				stored={on}
				disabled={!canEdit}
				commit={async (value) => {
					setArmed(value);
					if (!value && slot !== undefined) {
						return send(slotUnmarkOperations(itemId));
					}
					return { ok: true };
				}}
			/>
			{on && (
				<>
					<TextField
						label={t("ui.editor.template.name")}
						stored={slot?.name ?? ""}
						disabled={!canEdit || slot !== undefined}
						maxLength={40}
						commit={async (name) =>
							name === "" ? { ok: true } : mark({ name })
						}
					/>
					<TextField
						label={t("ui.editor.template.description")}
						stored={slot?.description ?? ""}
						disabled={!canEdit || slot === undefined}
						maxLength={200}
						commit={(description) => mark({ description })}
					/>
					<ToggleField
						label={t("ui.editor.template.required")}
						stored={slot?.required ?? true}
						disabled={!canEdit || slot === undefined}
						commit={(required) => mark({ required })}
					/>
					{media && (
						<SelectField
							label={t("ui.editor.template.policy")}
							stored={slot?.duration_policy ?? "fit"}
							disabled={!canEdit || slot === undefined}
							options={POLICIES.map((policy) => ({
								value: policy,
								label: t(`ui.editor.template.policies.${policy}`),
							}))}
							commit={async (value) =>
								value === "fit" || value === "stretch_timeline"
									? mark({ durationPolicy: value })
									: { ok: true }
							}
						/>
					)}
				</>
			)}
		</Section>
	);
}
