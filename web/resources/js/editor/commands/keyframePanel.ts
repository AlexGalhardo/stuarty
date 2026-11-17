import { announce } from "../a11y/announce";
import type { EditorRuntime } from "../state/runtime";

// Alt+K (SDD-012 section 18.7): shows the keyframes of the selected item and
// hides them again. The section is the one of the inspector (`name="keyframes"`),
// so the key works the header button: it opens the inspector if it is hidden,
// expands the section and moves focus to it; pressed again from inside the
// section, it collapses it.
export function toggleKeyframePanel(runtime: EditorRuntime): void {
	const { ui, uiWriter } = runtime;
	if (ui.getState().selection.items.length === 0) return;
	if (!ui.getState().panels.inspector) uiWriter.toggleInspector();
	// The section lives in a tab of the strip (SDD-012 section 6.4).
	uiWriter.showInspectorTab("video");
	requestAnimationFrame(() => {
		const section = document.querySelector<HTMLElement>(
			'[data-section="keyframes"]',
		);
		const header = section?.querySelector<HTMLButtonElement>(
			"button[aria-expanded]",
		);
		if (section === null || section === undefined || !header) return;
		const expanded = header.getAttribute("aria-expanded") === "true";
		const inside = section.contains(document.activeElement);
		if (expanded && inside) {
			header.click();
			announce("ui.editor.announce.keyframes_hidden");
			return;
		}
		if (!expanded) header.click();
		header.focus();
		announce("ui.editor.announce.keyframes_shown");
	});
}
