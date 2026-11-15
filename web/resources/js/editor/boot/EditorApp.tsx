import { type ReactNode, useEffect } from "react";

import { TooltipProvider } from "@/components/ui/tooltip";
import { I18nProvider } from "@/lib/i18n";
import EditorShell from "../layout/EditorShell";
import { EditorProvider } from "../state/EditorContext";
import type { EditorRuntime } from "../state/runtime";
import type { EditorBootstrap } from "./bootstrap";
import { startDeepLink } from "./deepLink";

// The root of the editor: the translation context from the bootstrap, the
// runtime, and the boot sequence of SDD-012 section 5.6 (load the document,
// open the stream).
export default function EditorApp({
	bootstrap,
	runtime,
}: {
	bootstrap: EditorBootstrap;
	runtime: EditorRuntime;
}): ReactNode {
	useEffect(() => {
		void runtime.start();
		const stopDeepLink = startDeepLink(runtime);
		const onVisible = (): void => {
			if (document.visibilityState === "visible") runtime.checkStale();
		};
		const onOnline = (): void => runtime.checkStale();
		document.addEventListener("visibilitychange", onVisible);
		window.addEventListener("online", onOnline);
		return () => {
			document.removeEventListener("visibilitychange", onVisible);
			window.removeEventListener("online", onOnline);
			stopDeepLink();
			runtime.dispose();
		};
	}, [runtime]);

	return (
		<I18nProvider messages={bootstrap.messages} locale={bootstrap.locale}>
			<EditorProvider runtime={runtime}>
				<TooltipProvider>
					<EditorShell />
				</TooltipProvider>
			</EditorProvider>
		</I18nProvider>
	);
}
