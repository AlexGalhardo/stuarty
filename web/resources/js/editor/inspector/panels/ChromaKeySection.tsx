import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { chromaKeyOperations } from "../../commands/builders";
import { registryRange } from "../../commands/registryLimits";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../../state/EditorContext";
import ColorField from "../fields/ColorField";
import SliderField from "../fields/SliderField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// The colour a key starts with: the green of a green screen.
const DEFAULT_KEY_COLOR = "#00FF00";

// The Chroma key section of a clip (VV-015): the switch, the colour to remove
// and the three settings of `clip.set_chroma_key`, one command per change. The
// preview and the export draw the key from the plan, not from these values.
export default function ChromaKeySection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("clip.set_chroma_key");
	const runtime = useRuntime();
	const picking = useUiState((s) => s.eyedropper?.item === itemId);
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (!exposed || item === undefined) return null;
	const key = item.chroma_key;
	return (
		<Section title={t("ui.editor.clip.chroma_key_title")}>
			<ToggleField
				label={t("ui.editor.clip.chroma_key_on")}
				stored={key !== undefined}
				disabled={!canEdit}
				commit={(on) =>
					send(chromaKeyOperations(itemId, on ? DEFAULT_KEY_COLOR : null))
				}
			/>
			{key !== undefined && (
				<>
					<ColorField
						label={t("ui.editor.clip.chroma_key_color")}
						stored={key.color}
						disabled={!canEdit}
						commit={(color) => send(chromaKeyOperations(itemId, color))}
					/>
					<div className="flex flex-col gap-1">
						<Button
							type="button"
							size="sm"
							variant="outline"
							disabled={!canEdit}
							aria-pressed={picking}
							onClick={() =>
								runtime.uiWriter.setEyedropper(
									picking
										? null
										: {
												item: itemId,
												onPick: (hex) =>
													void send(chromaKeyOperations(itemId, hex)),
											},
								)
							}
						>
							{t("ui.editor.clip.chroma_pick")}
						</Button>
						{picking && (
							<p role="status" className="text-xs text-muted-foreground">
								{t("ui.editor.clip.chroma_pick_hint")}
							</p>
						)}
					</div>
					<SliderField
						label={t("ui.editor.clip.chroma_similarity")}
						stored={String(key.similarity)}
						disabled={!canEdit}
						{...registryRange("clip.set_chroma_key", "similarity")}
						step={0.01}
						commit={(similarity) =>
							send(chromaKeyOperations(itemId, key.color, { similarity }))
						}
					/>
					<SliderField
						label={t("ui.editor.clip.chroma_smoothness")}
						stored={String(key.smoothness)}
						disabled={!canEdit}
						{...registryRange("clip.set_chroma_key", "smoothness")}
						step={0.01}
						commit={(smoothness) =>
							send(chromaKeyOperations(itemId, key.color, { smoothness }))
						}
					/>
					<SliderField
						label={t("ui.editor.clip.chroma_spill")}
						stored={String(key.spill)}
						disabled={!canEdit}
						{...registryRange("clip.set_chroma_key", "spill")}
						step={0.01}
						commit={(spill) =>
							send(chromaKeyOperations(itemId, key.color, { spill }))
						}
					/>
				</>
			)}
		</Section>
	);
}
