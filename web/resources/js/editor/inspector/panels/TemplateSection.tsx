import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	nextSlotName,
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
// caption item: the "Template slot" toggle, which marks the item at once with
// a default name (VV-071), and the name (a rename is unmark and mark), description, required
// and the duration policy of a media slot. Every change is one `slot.mark`
// carrying the whole slot (re-marking with the same name updates it), the
// toggle off is `slot.unmark`. The core refuses another name on an item that
// already has one, so a rename sends the unmark and the new mark together.
export default function TemplateSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const items = useDocumentState((s) => s.document?.items);
	if (item === undefined) return null;
	const slot = item.slot;
	const media = item.type === "clip" || item.type === "audio";
	const on = slot !== undefined;

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
					if (!value) {
						return slot === undefined
							? { ok: true }
							: send(slotUnmarkOperations(itemId));
					}
					if (slot !== undefined) return { ok: true };
					const taken = Object.values(items ?? {}).map(
						(other) => other.slot?.name,
					);
					return mark({ name: nextSlotName(taken) });
				}}
			/>
			{on && (
				<>
					<TextField
						label={t("ui.editor.template.name")}
						stored={slot?.name ?? ""}
						disabled={!canEdit}
						maxLength={40}
						commit={async (name) => {
							if (name === "" || name === slot?.name) return { ok: true };
							// The core refuses another name on a marked item, so a rename
							// is unmark and mark in one batch.
							return slot === undefined
								? mark({ name })
								: send([
										...slotUnmarkOperations(itemId),
										...slotMarkOperations(itemId, {
											name,
											description: slot.description ?? "",
											required: slot.required ?? true,
											...(media
												? { durationPolicy: slot.duration_policy ?? "fit" }
												: {}),
										}),
									]);
						}}
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
