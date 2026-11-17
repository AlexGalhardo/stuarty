import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { shadowOperations } from "../../commands/builders";
import { registryLimits } from "../../commands/registryLimits";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import ColorField from "../fields/ColorField";
import NumberField from "../fields/NumberField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

type Setting = "dx" | "dy" | "blur" | "opacity";

const ROWS: readonly { key: Setting; step: number; suffix?: string }[] = [
	{ key: "dx", step: 1, suffix: "px" },
	{ key: "dy", step: 1, suffix: "px" },
	{ key: "blur", step: 1, suffix: "px" },
	{ key: "opacity", step: 0.05 },
];

// The Shadow section of a clip: the switch and the settings of
// `clip.set_shadow`, one command per change (settings not sent keep their
// value; null removes it). Preview and export draw it from the plan.
export default function ShadowSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("clip.set_shadow");
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (!exposed || item === undefined) return null;
	const shadow = item.shadow;
	return (
		<Section title={t("ui.editor.clip.shadow_title")}>
			<ToggleField
				label={t("ui.editor.clip.shadow_on")}
				stored={shadow !== undefined}
				disabled={!canEdit}
				commit={(on) => send(shadowOperations(itemId, on ? {} : null))}
			/>
			{shadow !== undefined && (
				<>
					<ColorField
						label={t("ui.editor.clip.shadow_color")}
						stored={shadow.color}
						disabled={!canEdit}
						commit={(color) => send(shadowOperations(itemId, { color }))}
					/>
					{ROWS.map(({ key, step, suffix }) => (
						<NumberField
							key={key}
							label={t(`ui.editor.clip.shadow_${key}`)}
							stored={String(shadow[key])}
							disabled={!canEdit}
							step={step}
							suffix={suffix}
							{...registryLimits("clip.set_shadow", "shadow", key)}
							commit={(value) =>
								send(shadowOperations(itemId, { [key]: value }))
							}
						/>
					))}
				</>
			)}
		</Section>
	);
}
