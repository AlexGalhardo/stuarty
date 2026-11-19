import { useEffect } from "react";
import { useRuntime } from "../state/EditorContext";
import type { PreviewSize } from "../state/uiStore";
import { toggleKeyframePanel } from "./keyframePanel";
import { rowFor } from "./keymapStore";
import { inOverlay, isEditable } from "./keyTarget";
import { openImport } from "./openImport";
import { openSpeedCurve } from "./speedCurvePanel";

const PREVIEW_SIZES: readonly PreviewSize[] = ["fit", "50", "100"];

// The keys of SDD-012 section 18.7 that act on a surface and not on an item,
// all rows of the registry: the preview size rows size the preview, `import`
// goes to the import control of the library and `save` says the project is
// saved: every command is saved when it is sent (RF-001), so the key changes
// nothing. The `export` row opens the export dialog and is read in
// `export/ExportButton.tsx`, where the dialog's state is. They are captured
// only outside an editable field and an open dialog, so a field or the browser
// keeps them there (the browser rules of section 18.7).
export function useShellShortcuts(): void {
	const runtime = useRuntime();
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (isEditable(event.target) || inOverlay(event.target)) return;
			const row = rowFor(event, ["global"]);
			if (row === null) return;
			const { uiWriter, noticeWriter, ui } = runtime;
			switch (row) {
				case "keyframe_panel":
					toggleKeyframePanel(runtime);
					break;
				case "speed_curve":
					openSpeedCurve(runtime);
					break;
				case "preview_fit":
				case "preview_zoom_in":
				case "preview_zoom_out": {
					const at = PREVIEW_SIZES.indexOf(ui.getState().previewSize);
					const next: PreviewSize | undefined =
						row === "preview_fit"
							? "fit"
							: PREVIEW_SIZES[
									row === "preview_zoom_in"
										? Math.min(at + 1, PREVIEW_SIZES.length - 1)
										: Math.max(at - 1, 0)
								];
					if (next === undefined) return;
					uiWriter.setPreviewSize(next);
					break;
				}
				case "import":
					openImport(runtime);
					break;
				case "save":
					noticeWriter.push({
						kind: "info",
						message: "",
						messageKey: "ui.editor.shortcuts.saved",
					});
					break;
				default:
					return;
			}
			event.preventDefault();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [runtime]);
}
