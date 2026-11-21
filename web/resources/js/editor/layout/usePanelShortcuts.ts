import { useEffect } from "react";

import { jumpLandmark } from "../a11y/focus";
import { rowFor } from "../commands/keymapStore";
import { useRuntime } from "../state/EditorContext";

// The panel and landmark rows of the registry (SDD-012 sections 18.2 and 18.9):
// `panel_library`, `panel_inspector` and `panel_history` toggle the library,
// inspector and history panels (the history is a sidebar of its own), `next_landmark` and `prev_landmark` jump between
// regions. They work from any focus, as the browser does not type anything with
// Alt+digit or F6. Opening History moves focus to its first row.
export function usePanelShortcuts(): void {
	const runtime = useRuntime();
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			const row = rowFor(event, ["global"]);
			if (row === "next_landmark" || row === "prev_landmark") {
				event.preventDefault();
				jumpLandmark(row === "next_landmark" ? 1 : -1);
				return;
			}
			if (
				row !== "panel_library" &&
				row !== "panel_inspector" &&
				row !== "panel_history"
			)
				return;
			event.preventDefault();
			const { uiWriter } = runtime;
			if (row === "panel_inspector") uiWriter.toggleInspector();
			else if (row === "panel_library") uiWriter.toggleLeftRegion();
			else uiWriter.toggleHistory();
			if (row === "panel_history" && runtime.ui.getState().panels.history) {
				requestAnimationFrame(() =>
					document
						.querySelector<HTMLElement>("[data-revision] button")
						?.focus(),
				);
			}
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [runtime]);
}
