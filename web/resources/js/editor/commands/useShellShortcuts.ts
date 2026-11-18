import { useEffect } from "react";
import { useRuntime } from "../state/EditorContext";
import type { PreviewSize } from "../state/uiStore";
import { toggleKeyframePanel } from "./keyframePanel";
import { hasMod } from "./keymap";
import { openImport } from "./openImport";
import { openSpeedCurve } from "./speedCurvePanel";

const PREVIEW_SIZES: readonly PreviewSize[] = ["fit", "50", "100"];

function isEditable(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

function inOverlay(target: EventTarget | null): boolean {
	return (
		target instanceof HTMLElement &&
		target.closest('[role="dialog"], [role="alertdialog"], [role="menu"]') !==
			null
	);
}

// The keys of SDD-012 section 18.7 that act on a surface and not on an item:
// Alt+Shift+Z, Alt+Shift+= and Alt+Shift+- size the preview (the physical key,
// as Option changes the character on a Mac), Mod+I goes to the import field of
// the library and Mod+S says the project is saved: every command is saved when
// it is sent (RF-001), so the key changes nothing. Mod+E opens the export
// dialog and is read in `export/ExportButton.tsx`, where the dialog's state is.
// They are captured only outside an editable field and an open dialog, so a
// field or the browser keeps them there (the browser rules of section 18.7).
export function useShellShortcuts(): void {
	const runtime = useRuntime();
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (isEditable(event.target) || inOverlay(event.target)) return;
			const { uiWriter, noticeWriter, ui } = runtime;
			if (event.altKey && !event.shiftKey && !hasMod(event)) {
				// The physical key: Option changes the character on a Mac.
				if (event.code === "KeyX") {
					event.preventDefault();
					uiWriter.clearRange();
				} else if (event.code === "KeyK") {
					event.preventDefault();
					toggleKeyframePanel(runtime);
				}
				return;
			}
			if (event.altKey && event.shiftKey && !hasMod(event)) {
				const current = ui.getState().previewSize;
				const at = PREVIEW_SIZES.indexOf(current);
				let next: PreviewSize | null = null;
				if (event.code === "KeyZ") next = "fit";
				else if (event.code === "Equal")
					next =
						PREVIEW_SIZES[Math.min(at + 1, PREVIEW_SIZES.length - 1)] ?? null;
				else if (event.code === "Minus")
					next = PREVIEW_SIZES[Math.max(at - 1, 0)] ?? null;
				if (next === null) return;
				event.preventDefault();
				uiWriter.setPreviewSize(next);
				return;
			}
			if (event.shiftKey && !event.altKey && !hasMod(event)) {
				if (event.code === "KeyB") {
					event.preventDefault();
					openSpeedCurve(runtime);
				}
				return;
			}
			if (!hasMod(event) || event.altKey || event.shiftKey) return;
			const key = event.key.toLowerCase();
			if (key === "i") {
				event.preventDefault();
				openImport(runtime);
			} else if (key === "s") {
				event.preventDefault();
				noticeWriter.push({
					kind: "info",
					message: "",
					messageKey: "ui.editor.shortcuts.saved",
				});
			}
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [runtime]);
}
