import { type ReactNode, useEffect, useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";
import {
	canDropPaths,
	canPickFiles,
	getDesktopBridge,
	subscribeDesktopBridge,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import { dismissTips, rememberTips, useTipsVisible } from "../state/tipsStore";

const TIPS = ["library", "palette", "shortcuts"] as const;

// The first-run tips (SDD-012 section 18.5): a short list under the toolbar,
// shown once per browser and closed with "Got it". Showing them is what
// remembers them (`vv.shortcutsSeen`).
export default function FirstRunTips(): ReactNode {
	const t = useT();
	const visible = useTipsVisible();
	// In the desktop window the library also has "Choose files" and drops.
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const desktop = canPickFiles(bridge) || canDropPaths(bridge);
	useEffect(() => {
		if (visible) rememberTips();
	}, [visible]);
	if (!visible) return null;
	return (
		<section
			aria-label={t("ui.editor.tips.title")}
			className="flex items-center gap-4 border-b border-ed-track-line bg-ed-panel px-3 py-2 text-sm"
		>
			<h2 className="font-semibold">{t("ui.editor.tips.title")}</h2>
			<ul className="flex flex-1 flex-wrap gap-x-4 gap-y-1 text-muted-foreground">
				{TIPS.map((tip) => (
					<li key={tip}>
						{t(
							tip === "library" && desktop
								? "ui.editor.tips.items.library_desktop"
								: `ui.editor.tips.items.${tip}`,
						)}
					</li>
				))}
			</ul>
			<Button type="button" size="sm" variant="outline" onClick={dismissTips}>
				{t("ui.editor.tips.dismiss")}
			</Button>
		</section>
	);
}
