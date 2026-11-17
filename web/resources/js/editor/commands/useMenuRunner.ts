import { useCallback } from "react";

import { useT } from "@/lib/i18n";
import { useRuntime } from "../state/EditorContext";
import { runMenuAction } from "./menu";
import { useActions } from "./useActions";

// The function that runs a menu action by id: the web menu calls it from an
// item, the desktop shell from `onMenuAction`.
export function useMenuRunner(): (id: string) => boolean {
	const actions = useActions();
	const runtime = useRuntime();
	const t = useT();
	return useCallback(
		(id) => runMenuAction(id, { actions, runtime, t }),
		[actions, runtime, t],
	);
}
