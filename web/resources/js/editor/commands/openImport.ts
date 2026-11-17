import type { EditorRuntime } from "../state/runtime";

// Mod+I and the menu: shows the library and goes to its import control, the
// native chooser of the desktop shell or else the path field.
export function openImport(runtime: EditorRuntime): void {
	runtime.uiWriter.showLeftTab("library");
	requestAnimationFrame(() => {
		const choose = document.querySelector<HTMLElement>("[data-import-choose]");
		if (choose !== null) choose.click();
		else document.querySelector<HTMLElement>("[data-import-path]")?.focus();
	});
}
