import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	BLEND_MODES,
	type BlendMode,
	blendOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import SelectField from "../fields/SelectField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const isBlend = (value: string): value is BlendMode =>
	(BLEND_MODES as readonly string[]).includes(value);

// The Blend section of a visual item (VV-015): how it mixes with what is
// below it, `clip.set_blend`. The preview and the export draw it from the plan.
export default function BlendSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("clip.set_blend");
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (!exposed || item === undefined) return null;
	return (
		<Section title={t("ui.editor.clip.blend_title")}>
			<SelectField
				label={t("ui.editor.clip.blend_mode")}
				stored={item.blend ?? "normal"}
				disabled={!canEdit}
				options={BLEND_MODES.map((mode) => ({
					value: mode,
					label: t(`ui.editor.clip.blend_${mode}`),
				}))}
				commit={async (value) =>
					isBlend(value) ? send(blendOperations(itemId, value)) : { ok: true }
				}
			/>
		</Section>
	);
}
