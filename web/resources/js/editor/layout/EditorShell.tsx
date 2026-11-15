import { type ReactNode, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import Library from "../library/Library";
import Preview from "../preview/Preview";
import { useDocumentState } from "../state/EditorContext";
import EditToolbar from "../timeline/EditToolbar";
import Notices from "../timeline/Notices";
import Timeline from "../timeline/Timeline";
import { useShortcuts } from "../timeline/useShortcuts";
import TopBar from "./TopBar";

// Panel sizes by viewport width (SDD-012 section 6.3); below 1280 the drawers
// of the later phases are not built yet, so the compact sizes apply.
export interface PanelSizes {
	library: number;
	inspector: number;
	timeline: number;
}

export function panelSizes(viewportWidth: number): PanelSizes {
	if (viewportWidth >= 1600)
		return { library: 320, inspector: 360, timeline: 300 };
	return { library: 240, inspector: 280, timeline: 240 };
}

const MIN_SUPPORTED_WIDTH = 1024;

function useViewportWidth(): number {
	const [width, setWidth] = useState(() => window.innerWidth);
	useEffect(() => {
		const onResize = (): void => setWidth(window.innerWidth);
		window.addEventListener("resize", onResize);
		return () => window.removeEventListener("resize", onResize);
	}, []);
	return width;
}

function Panel({
	label,
	children,
}: {
	label: string;
	children?: ReactNode;
}): ReactNode {
	const t = useT();
	return (
		<section
			aria-label={label}
			className="flex min-h-0 min-w-0 flex-col overflow-auto bg-ed-panel p-3"
		>
			<h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
				{label}
			</h2>
			{children ?? (
				<p className="mt-2 text-sm text-muted-foreground">
					{t("ui.editor.panels.soon")}
				</p>
			)}
		</section>
	);
}

// The grid of SDD-012 section 6.1: top bar, library, preview, inspector and
// the timeline across the bottom. The library, the preview and the inspector
// are filled by their own tasks; here they are labelled regions.
export default function EditorShell(): ReactNode {
	const t = useT();
	const status = useDocumentState((s) => s.status);
	const viewport = useViewportWidth();
	const sizes = panelSizes(viewport);
	const [captionOpen, setCaptionOpen] = useState(false);
	useShortcuts(() => setCaptionOpen(true));

	if (viewport < MIN_SUPPORTED_WIDTH) {
		return (
			<main className="flex h-dvh flex-col items-center justify-center gap-4 p-6 text-center">
				<p>{t("ui.editor.unsupported_size")}</p>
				<Button asChild variant="outline">
					<a href="/projects">{t("ui.editor.back")}</a>
				</Button>
			</main>
		);
	}

	return (
		<div
			className="grid h-dvh min-h-0 bg-ed-bg text-foreground"
			style={{
				gridTemplateRows: `auto minmax(0, 1fr) ${sizes.timeline}px`,
				gridTemplateColumns: `${sizes.library}px minmax(0, 1fr) ${sizes.inspector}px`,
			}}
		>
			<div className="col-span-full">
				<TopBar />
				<div className="border-b border-ed-track-line bg-ed-panel px-3 py-1">
					<EditToolbar
						captionOpen={captionOpen}
						onCaptionOpenChange={setCaptionOpen}
					/>
				</div>
				{status === "failed" && (
					<div
						role="alert"
						className="col-span-full flex items-center gap-3 border-b border-destructive bg-ed-panel px-3 py-2 text-sm text-destructive"
					>
						{t("ui.editor.banner.failed")}
						<Button
							type="button"
							size="sm"
							variant="outline"
							onClick={() => location.reload()}
						>
							{t("ui.editor.banner.reload")}
						</Button>
					</div>
				)}
			</div>
			<Library />
			<main
				aria-label={t("ui.editor.panels.preview")}
				className="flex min-h-0 min-w-0 flex-col bg-ed-preview-surround p-3"
			>
				{status === "loading" ? (
					<p role="status" className="text-sm text-muted-foreground">
						{t("ui.editor.loading")}
					</p>
				) : (
					<Preview />
				)}
			</main>
			<Panel label={t("ui.editor.panels.inspector")} />
			<div className="col-span-full min-h-0 border-t border-ed-track-line">
				<Timeline />
			</div>
			<Notices />
		</div>
	);
}
