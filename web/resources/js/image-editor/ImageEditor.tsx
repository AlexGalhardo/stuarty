import {
	CircleAlert,
	Image as ImageIcon,
	ImagePlus,
	Loader2,
} from "lucide-react";
import { type ReactNode, useEffect, useMemo, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { browserTimers } from "../editor/api/events";
import { HttpTransport } from "../editor/api/http";
import { useShell } from "../shell/ShellContext";
import { API_BASE, createImageApi } from "./api/imageApi";
import { createImageEventStream } from "./api/imageEvents";
import Canvas from "./canvas/Canvas";
import ErrorBar from "./ErrorBar";
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
import { AddImageProvider, useAddImage } from "./toolbar/addImage";
import { Toolbar, ViewTools } from "./toolbar/Toolbar";

// The editor takes the room the frame of the page leaves (FullLayout): the window less the top bar of the
// editor and the scene bar. The floor keeps a short window from crushing the canvas.
export const EDITOR_FRAME = "flex min-h-0 flex-1 flex-col bg-ed-bg";

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
	// Whether the event stream is open: the top bar says "Live" only then (polling is not live).
	const [live, setLive] = useState(false);

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
		const setStreamLive = (now: boolean): void => {
			live = now;
			setLive(now);
		};

		// Opens the stream once the document is on screen, with the revision it holds.
		const start = (): void => {
			if (disposed || !visible() || store.getState().doc === null) return;
			stream.open(revision());
		};
		const reload = (): void => {
			void store.getState().open().then(start);
		};

		const offState = stream.onState((state) => {
			setStreamLive(state === "live");
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
				setStreamLive(false);
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
				<AddImageProvider>
					<Shell live={live} />
				</AddImageProvider>
			</LibraryProvider>
		</ImageEditorProvider>
	);
}

function Shell({ live }: { live: boolean }): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const load = useImage((s) => s.load);
	const error = useImage((s) => s.error);
	const open = useImage((s) => s.open);
	const onKeys = useEditorKeys();
	const failed = load === "failed";

	return (
		<section
			aria-labelledby="image-title"
			data-testid="image-editor"
			onKeyDown={onKeys}
			className={EDITOR_FRAME}
		>
			<TopBar doc={doc} failed={failed} live={live} />
			<AgentNotice />
			<ModelJobStrip />
			{!failed && <ErrorBar />}
			<div className="flex min-h-0 flex-1">
				<Toolbar label={t("ui.image.toolbar.label")} orientation="vertical">
					<HistoryTools />
					<AddTools />
					<ArrangeTools />
					<ModelEditTools />
				</Toolbar>
				<aside
					aria-label={t("ui.image.layers.title")}
					className="flex w-56 shrink-0 flex-col border-r border-border bg-ed-panel xl:w-60"
				>
					<LayerPanel />
				</aside>
				<div className="relative flex min-w-0 flex-1 flex-col">
					<div className="relative min-h-0 flex-1">
						<Canvas />
						{failed && (
							<StateCard>
								<div
									role="alert"
									data-testid="image-load-failed"
									className="flex items-start gap-3 rounded-lg bg-danger-soft p-4 text-danger-text"
								>
									<CircleAlert
										aria-hidden="true"
										className="mt-0.5 size-5 shrink-0"
									/>
									<div className="min-w-0 flex-1">
										<h2 className="m-0 text-sm font-semibold">
											{t("ui.image.editor.failed")}
										</h2>
										{error !== null && (
											<p lang="en" className="mt-1 text-sm">
												<UntrustedText>{error.message}</UntrustedText>
											</p>
										)}
									</div>
									<Button
										type="button"
										variant="outline"
										size="sm"
										className="pointer-events-auto"
										onClick={() => void open()}
									>
										{t("ui.image.canvas.retry")}
									</Button>
								</div>
							</StateCard>
						)}
						{!failed && doc === null && (
							<StateCard>
								<p
									role="status"
									data-testid="image-loading"
									className="flex items-center gap-2 text-sm text-muted-foreground"
								>
									<Loader2 aria-hidden="true" className="size-4 animate-spin" />
									{t("ui.image.editor.loading")}
								</p>
							</StateCard>
						)}
						{doc !== null && doc.layers.length === 0 && <EmptyDocument />}
					</div>
					<Toolbar
						label={t("ui.image.toolbar.view")}
						testId="image-view-toolbar"
						className="justify-center border-t border-border"
					>
						<ViewTools />
					</Toolbar>
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

// What the stage says when there is no picture to draw: the loading, the failure, the empty document.
function StateCard({ children }: { children: ReactNode }): ReactNode {
	return (
		<div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center p-6">
			<div className="flex max-w-xl flex-col items-center">{children}</div>
		</div>
	);
}

// A document with no layer (the empty state of the final design): three steps and the two ways to begin, a
// text layer here or a request to the agent.
function EmptyDocument(): ReactNode {
	const t = useT();
	const busy = useImage((s) => s.busy);
	const { setPicking } = useAddImage();
	const { openChat } = useShell();
	const steps = ["photo", "title", "background"] as const;
	return (
		<StateCard>
			<div
				data-testid="image-empty"
				className="pointer-events-none w-full max-w-lg rounded-xl border border-dashed border-border-strong bg-surface-raised p-6"
			>
				<div className="flex items-start gap-4">
					<span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent-text">
						<ImageIcon aria-hidden="true" className="size-5" />
					</span>
					<div>
						<h2 className="m-0 text-lg font-semibold">
							{t("ui.image.empty.title")}
						</h2>
						<p className="mt-1 text-sm text-muted-foreground">
							{t("ui.image.empty.lead")}
						</p>
					</div>
				</div>
				<ol className="mt-4 flex list-none flex-col gap-3 p-0">
					{steps.map((step, index) => (
						<li key={step} className="flex items-start gap-3">
							<span
								aria-hidden="true"
								className="flex size-6 shrink-0 items-center justify-center rounded-full bg-accent-soft text-xs font-semibold text-accent-text"
							>
								{index + 1}
							</span>
							<span className="text-sm">
								<span className="block font-medium">
									{t(`ui.image.empty.step_${step}`)}
								</span>
								<span className="block text-muted-foreground">
									{t(`ui.image.empty.step_${step}_body`)}
								</span>
							</span>
						</li>
					))}
				</ol>
				<div className="mt-5 flex items-center gap-2">
					<Button
						type="button"
						className="pointer-events-auto"
						disabled={busy}
						data-testid="image-empty-add-photo"
						onClick={() => setPicking(true)}
					>
						<ImagePlus aria-hidden="true" />
						{t("ui.image.empty.add_photo")}
					</Button>
					<Button
						type="button"
						variant="ghost"
						className="pointer-events-auto"
						onClick={openChat}
					>
						{t("ui.image.empty.ask_agent")}
					</Button>
				</div>
			</div>
		</StateCard>
	);
}
