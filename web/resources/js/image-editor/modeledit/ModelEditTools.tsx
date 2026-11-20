import { Eraser, Sparkles } from "lucide-react";
import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import type { ModelOperation } from "../api/modelEdit";
import { findLayer, useImage } from "../state/context";
import ToolButton from "../toolbar/ToolButton";
import { ToolGroup } from "../toolbar/Toolbar";
import { ModelEditDialog } from "./ModelEditDialog";
import { isFinal, useModelJob } from "./state";

// The two buttons of the operations with a model (SDD-016 section 6.5, SDD-012 section 24.14): "Remove
// background" for the selected image layer, and "Generate" for a new layer from a prompt. Each opens the dialog
// that shows what is sent and the estimate; a job already running disables both, so one is followed at a time.
export function ModelEditTools(): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const busy = useImage((s) => s.busy);
	const selection = useImage((s) => s.selection);
	const running = useModelJob(
		(s) => s.tracked !== null && !isFinal(s.tracked.job),
	);
	const [open, setOpen] = useState<ModelOperation | null>(null);

	const only = selection.length === 1 ? selection[0] : undefined;
	const layer =
		doc === null || only === undefined
			? undefined
			: findLayer(doc.layers, only);
	// A removal needs one image layer that has a file; the service checks the rest (a locked layer, a ready file).
	const removable =
		layer !== undefined && layer.kind === "image" && layer.asset !== undefined;
	// A generation may cover the selected layer when it has a box of its own (a group has none).
	const coverable = layer !== undefined && layer.kind !== "group";
	const off = doc === null || busy || running;

	return (
		<ToolGroup>
			<ToolButton
				icon={Eraser}
				label={
					removable
						? t("ui.image.model.remove")
						: t("ui.image.model.remove_needs_layer")
				}
				disabled={off || !removable}
				onClick={() => setOpen("remove_background")}
				testId="image-model-remove"
			/>
			<ToolButton
				icon={Sparkles}
				label={t("ui.image.model.generate")}
				disabled={off}
				onClick={() => setOpen("generate")}
				testId="image-model-generate"
			/>
			{open !== null && (
				<ModelEditDialog
					operation={open}
					layerId={coverable ? (layer?.id ?? null) : null}
					open
					onOpenChange={(next) => {
						if (!next) setOpen(null);
					}}
				/>
			)}
		</ToolGroup>
	);
}
