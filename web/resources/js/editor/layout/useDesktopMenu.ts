import { useEffect, useSyncExternalStore } from "react";

import {
	canMenuAction,
	getDesktopBridge,
	subscribeDesktopBridge,
} from "@/lib/desktopBridge";
import { useMenuRunner } from "../commands/useMenuRunner";

// The native menu bar of the desktop shell (SDD-012 section 21.3 item 4): a
// click arrives as an action id and runs the same action as the item of the
// web menu. A plain browser has no bridge and nothing is subscribed; the shell
// can define the bridge after the first render, so the subscription follows it.
export function useDesktopMenu(): void {
	const run = useMenuRunner();
	const present = useSyncExternalStore(
		subscribeDesktopBridge,
		() => canMenuAction(getDesktopBridge()),
		() => false,
	);
	useEffect(() => {
		if (!present) return;
		return getDesktopBridge()?.onMenuAction?.((action) => void run(action));
	}, [present, run]);
}
