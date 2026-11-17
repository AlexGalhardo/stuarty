import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { outlineOperations } from "../../commands/builders";
import { registryLimits } from "../../commands/registryLimits";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import ColorField from "../fields/ColorField";
import NumberField from "../fields/NumberField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// The Outline section of a clip: the switch, the colour and the width of
// `clip.set_outline`, one command per change (null removes it). Preview and
// export draw it from the plan.
export default function OutlineSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("clip.set_outline");
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (!exposed || item === undefined) return null;
	const outline = item.outline;
	return (
		<Section title={t("ui.editor.clip.outline_title")}>
			<ToggleField
				label={t("ui.editor.clip.outline_on")}
				stored={outline !== undefined}
				disabled={!canEdit}
				commit={(on) => send(outlineOperations(itemId, on ? {} : null))}
			/>
			{outline !== undefined && (
				<>
					<ColorField
						label={t("ui.editor.clip.outline_color")}
						stored={outline.color}
						disabled={!canEdit}
						commit={(color) => send(outlineOperations(itemId, { color }))}
					/>
					<NumberField
						label={t("ui.editor.clip.outline_width")}
						stored={String(outline.width)}
						disabled={!canEdit}
						step={1}
						suffix="px"
						{...registryLimits("clip.set_outline", "outline", "width")}
						commit={(width) => send(outlineOperations(itemId, { width }))}
					/>
				</>
			)}
		</Section>
	);
}
