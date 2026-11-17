import { announce } from "../a11y/announce";
import type { EditorRuntime } from "../state/runtime";

// Shift+B (SDD-012 section 18.7): opens the speed curve editor of the selected
// clip. It opens the inspector if it is hidden, expands the Speed section,
// starts a draft curve when the clip has none (a draft is not a command: it is
// sent only by Apply) and moves focus to the first field that can be edited. An item without the
// Speed section (text, caption, audio) leaves the key without effect.
export function openSpeedCurve(runtime: EditorRuntime): void {
	const { ui, uiWriter } = runtime;
	if (ui.getState().selection.items.length === 0) return;
	if (!ui.getState().panels.inspector) uiWriter.toggleInspector();
	// The section lives in a tab of the strip (SDD-012 section 6.4).
	uiWriter.showInspectorTab("speed");
	requestAnimationFrame(() => {
		const section = document.querySelector<HTMLElement>(
			'[data-section="speed"]',
		);
		const header = section?.querySelector<HTMLButtonElement>(
			"button[aria-expanded]",
		);
		if (section === null || section === undefined || !header) return;
		if (header.getAttribute("aria-expanded") !== "true") header.click();
		section
			.querySelector<HTMLButtonElement>("[data-speed-curve-start]")
			?.click();
		requestAnimationFrame(() => {
			const field = section.querySelector<HTMLElement>(
				"[data-speed-curve] input:not([disabled])",
			);
			if (field === null) return;
			field.focus();
			announce("ui.editor.announce.speed_curve_shown");
		});
	});
}
