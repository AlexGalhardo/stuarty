import { Link } from "@inertiajs/react";
import { type ReactNode, useEffect, useMemo } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { browserTimers } from "../editor/api/events";
import { HttpTransport } from "../editor/api/http";
import { API_BASE, createImageApi } from "./api/imageApi";
import { createImageEventStream } from "./api/imageEvents";
import Canvas from "./canvas/Canvas";
import ErrorBar from "./ErrorBar";
import { ExportTool } from "./export/ExportDialog";
import AgentNotice from "./history/AgentNotice";
import HistoryPanel from "./history/HistoryPanel";
import Inspector from "./inspector/Inspector";
import { useEditorKeys } from "./keys";
import LayerPanel from "./layers/LayerPanel";
import { ModelEditTools } from "./modeledit/ModelEditTools";
import { ModelJobStrip } from "./modeledit/ModelJobStrip";
import { ImageEditorProvider, useImage } from "./state/context";
import { LibraryProvider } from "./state/library";
import { createImageStore } from "./state/store";
import TopBar from "./TopBar";
import { AddTools } from "./toolbar/AddTools";
import { ArrangeTools, HistoryTools } from "./toolbar/ArrangeTools";
import { Toolbar, ViewTools } from "./toolbar/Toolbar";

// The editor takes the room the frame of the dashboard leaves: the window less the top bar (3.5 rem), the scene
// bar at the foot (3.5 rem, always in view) and the padding of the main region (2 rem, 3 rem from the md
// breakpoint). Sized to the whole window the page scrolled, and a focus on the canvas moved the picture
// under the pointer between the press and the release of a click.
export const EDITOR_FRAME =
	"flex h-[calc(100dvh-9rem)] min-h-[28rem] flex-col overflow-hidden rounded-lg border border-border bg-ed-bg md:h-[calc(100dvh-10rem)]";

// How often the screen asks whether another actor changed the document while the event stream is not open
// (SDD-012 section 24.3).
export const POLL_MS = 2000;

// The image editor of one document (SDD-012 section 24): a toolbar, the canvas and, on the sides, the layer
// panel and the inspector. It sends commands and draws what the server answers.
export default function ImageEditor({
	imageId,
}: {
	imageId: string;
}): ReactNode {
	const transport = useMemo(() => new HttpTransport(API_BASE), []);
	const api = useMemo(() => createImageApi(transport), [transport]);
	const store = useMemo(() => createImageStore(api, imageId), [api, imageId]);

	// The stream of the image (SDD-006 section 12) tells this window what another actor or window committed. It is
	// open while the window is visible; while it is not open the revision is polled (SDD-012 section 24.3).
	useEffect(() => {
		const revision = (): number => store.getState().doc?.revision ?? 0;
		const visible = (): boolean => document.visibilityState === "visible";
		const stream = createImageEventStream({
			transport,
			imageId,
			getRevision: revision,
			probe: () => api.probe(imageId),
			timers: browserTimers,
		});
		let live = false;
		let disposed = false;

		// Opens the stream once the document is on screen, with the revision it holds.
		const start = (): void => {
			if (disposed || !visible() || store.getState().doc === null) return;
			stream.open(revision());
		};
		const reload = (): void => {
			void store.getState().open().then(start);
		};

		const offState = stream.onState((state) => {
			live = state === "live";
		});
		const offEvent = stream.onEvent((event) => {
			if (event.name === "resync.required") {
				// The server cannot replay: the stream ended there, so load the image again and open a new one.
				stream.close();
				reload();
				return;
			}
			void store.getState().receive(event);
		});
		const offRecover = stream.onRecover(reload);

		const poll = (): void => {
			if (visible() && !live) void store.getState().poll();
		};
		const onVisibility = (): void => {
			if (visible()) {
				start();
				poll();
			} else {
				stream.close();
				live = false;
			}
		};
		const onOnline = (): void => stream.checkStale();

		reload();
		const timer = window.setInterval(poll, POLL_MS);
		document.addEventListener("visibilitychange", onVisibility);
		window.addEventListener("online", onOnline);
		return () => {
			disposed = true;
			window.clearInterval(timer);
			document.removeEventListener("visibilitychange", onVisibility);
			window.removeEventListener("online", onOnline);
			offState();
			offEvent();
			offRecover();
			stream.close();
		};
	}, [store, transport, api, imageId]);

	return (
		<ImageEditorProvider store={store} api={api}>
			<LibraryProvider>
				<Shell />
			</LibraryProvider>
		</ImageEditorProvider>
	);
}

function Shell(): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const load = useImage((s) => s.load);
	const error = useImage((s) => s.error);
	const open = useImage((s) => s.open);
	const onKeys = useEditorKeys();

	if (load === "failed") {
		return (
			<section aria-labelledby="image-title" className="max-w-xl">
				<h1 id="image-title" className="text-xl font-semibold">
					{t("ui.image.editor.failed")}
				</h1>
				{error !== null && (
					<p lang="en" className="mt-2 text-sm">
						<UntrustedText>{error.message}</UntrustedText>
					</p>
				)}
				<div className="mt-3 flex gap-2">
					<Button type="button" onClick={() => void open()}>
						{t("ui.image.canvas.retry")}
					</Button>
					<Button asChild variant="outline">
						<Link href="/edit-image">{t("ui.image.editor.back")}</Link>
					</Button>
				</div>
			</section>
		);
	}

	return (
		<section
			aria-labelledby="image-title"
			data-testid="image-editor"
			onKeyDown={onKeys}
			className={EDITOR_FRAME}
		>
			<TopBar doc={doc} />
			<Toolbar label={t("ui.image.toolbar.label")}>
				<HistoryTools />
				<AddTools />
				<ArrangeTools />
				<ModelEditTools />
				<ViewTools />
				<ExportTool />
			</Toolbar>
			<AgentNotice />
			<ModelJobStrip />
			<ErrorBar />
			<div className="flex min-h-0 flex-1">
				<aside
					aria-label={t("ui.image.layers.title")}
					className="flex w-56 shrink-0 flex-col overflow-y-auto border-r border-border bg-ed-panel p-3 xl:w-64"
				>
					<LayerPanel />
				</aside>
				<div className="min-w-0 flex-1">
					<Canvas />
				</div>
				<aside
					aria-label={t("ui.image.inspector.title")}
					className="flex w-72 shrink-0 flex-col border-l border-border bg-ed-panel xl:w-80"
				>
					<Inspector />
					<HistoryPanel />
				</aside>
			</div>
		</section>
	);
}
