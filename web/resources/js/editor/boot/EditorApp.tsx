import { type ReactNode, useEffect } from "react";

import { TooltipProvider } from "@/components/ui/tooltip";
import { I18nProvider } from "@/lib/i18n";
import EditorSceneBar, { EditorShellFrame } from "../layout/EditorSceneBar";
import EditorShell from "../layout/EditorShell";
import { EditorProvider } from "../state/EditorContext";
import type { EditorRuntime } from "../state/runtime";
import type { EditorBootstrap } from "./bootstrap";
import { startDeepLink } from "./deepLink";
import { ErrorBoundary } from "./ErrorBoundary";
import { switchLocale } from "./locale";

// The agent session (the bell, Chat and the scene bar) is there only when the page has a session; the
// development fixture has none.
function WithAgent({
	bootstrap,
	children,
}: {
	bootstrap: EditorBootstrap;
	children: ReactNode;
}): ReactNode {
	if (bootstrap.session === undefined) return children;
	return (
		<EditorShellFrame
			projectId={bootstrap.project_id}
			privileged={bootstrap.session.privileged}
			mode={bootstrap.mode}
		>
			{children}
		</EditorShellFrame>
	);
}

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
		<I18nProvider
			messages={bootstrap.messages}
			locale={bootstrap.locale}
			locales={bootstrap.locales}
		>
			<ErrorBoundary>
				<EditorProvider runtime={runtime}>
					<TooltipProvider>
						<WithAgent bootstrap={bootstrap}>
							<EditorShell
								standalone={bootstrap.standalone === true}
								sceneBar={
									bootstrap.session === undefined ? null : <EditorSceneBar />
								}
								onLocaleChange={(next) => {
									const { playheadFrame, selection } = runtime.ui.getState();
									switchLocale(next, {
										frame: playheadFrame,
										item: selection.items[0] ?? null,
									});
								}}
							/>
						</WithAgent>
					</TooltipProvider>
				</EditorProvider>
			</ErrorBoundary>
		</I18nProvider>
	);
}
