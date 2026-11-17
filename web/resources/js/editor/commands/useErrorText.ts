import { useT } from "@/lib/i18n";
import { useOptionalRuntime } from "../state/EditorContext";
import { readTimeDisplay } from "../state/prefs";
import { selectFps, selectItem } from "../state/selectors";
import { type ErrorLike, type ErrorText, errorText } from "../ui/errorText";
import { formatTime, fpsOf } from "./units";

// The catalogue wording of an error in the language of the page (VV-126), with
// the item names and the time display of the open project.
export function useErrorText(): (error: ErrorLike) => ErrorText {
	const t = useT();
	const runtime = useOptionalRuntime();
	return (error) =>
		errorText(t, error, {
			itemName: (id) => {
				const state = runtime?.documents.getState();
				const item = state === undefined ? undefined : selectItem(state, id);
				if (item === undefined) return id;
				return item.name ?? t(`ui.editor.item.${item.type}`);
			},
			time: (frame) => {
				const state = runtime?.documents.getState();
				const fps = fpsOf(state === undefined ? undefined : selectFps(state));
				return formatTime(frame, fps, readTimeDisplay());
			},
		});
}
