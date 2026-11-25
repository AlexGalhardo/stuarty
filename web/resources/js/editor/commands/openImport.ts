import type { EditorRuntime } from "../state/runtime";

// Asks the library to show its import controls (Mod+I and the menu): they are folded under the button of its header.
export const OPEN_IMPORT_EVENT = "stuarty:open-import";

// Mod+I and the menu: shows the library and goes to its import control, the
// native chooser of the desktop shell or else the path field.
export function openImport(runtime: EditorRuntime): void {
	runtime.uiWriter.showLeftTab("library");
	requestAnimationFrame(() => {
		window.dispatchEvent(new Event(OPEN_IMPORT_EVENT));
		requestAnimationFrame(focusImport);
	});
}

function focusImport(): void {
	const choose = document.querySelector<HTMLElement>("[data-import-choose]");
	if (choose !== null) choose.click();
	else document.querySelector<HTMLElement>("[data-import-path]")?.focus();
}
