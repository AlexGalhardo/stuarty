import { useEffect } from "react";

import { jumpLandmark } from "../a11y/focus";
import { landmarkShortcut, panelShortcut } from "../commands/keymap";
import { useRuntime } from "../state/EditorContext";

// Alt+1, Alt+2 and Alt+3 toggle the library, inspector and history panels
// (SDD-012 section 18.2). They work from any focus, as the browser does not
// type anything with Alt+digit. Opening History moves focus to its first row.
export function usePanelShortcuts(): void {
	const runtime = useRuntime();
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			const direction = landmarkShortcut(event);
			if (direction !== null) {
				event.preventDefault();
				jumpLandmark(direction);
				return;
			}
			const panel = panelShortcut(event);
			if (panel === null) return;
			event.preventDefault();
			const { uiWriter } = runtime;
			if (panel === "inspector") uiWriter.toggleInspector();
			else if (panel === "library") uiWriter.toggleLeftRegion();
			else uiWriter.toggleLeftTab(panel);
			if (panel === "history" && runtime.ui.getState().panels.left) {
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
