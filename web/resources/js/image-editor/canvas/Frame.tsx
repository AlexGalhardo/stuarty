import { type ReactNode, useEffect, useState } from "react";

import { useT } from "@/lib/i18n";
import { frameUrl } from "../api/imageApi";

// The widths the picture is asked for: the stage asks the smallest that is at least as wide as the screen
// needs, so a zoom does not request a new picture at every step.
const WIDTHS = [256, 512, 768, 1024, 1280, 1600, 2048, 3072, 4096] as const;

const RETRIES = 2;
const RETRY_MS = 400;

export function frameWidth(needed: number, canvasWidth: number): number {
	if (needed >= canvasWidth) return 0;
	return WIDTHS.find((w) => w >= needed && w < canvasWidth) ?? 0;
}

// The picture of the document: the PNG of the current revision as the renderer draws it (SDD-012 section
// 24.4). A new picture is loaded off screen and swapped in when it is ready, so a change never blanks the canvas.
export default function Frame({
	imageId,
	revision,
	width,
	height,
	zoom,
}: {
	imageId: string;
	revision: number;
	width: number;
	height: number;
	zoom: number;
}): ReactNode {
	const t = useT();
	const dpr = typeof window === "undefined" ? 1 : window.devicePixelRatio || 1;
	const asked = frameWidth(Math.ceil(width * zoom * dpr), width);
	const wanted = frameUrl(imageId, revision, asked === 0 ? width : asked);
	// The picture on screen and the revision it was asked for: while the picture of a new revision loads, the old
	// one stays, and `data-revision` must name the one that is drawn, not the one that is wanted.
	const [shown, setShown] = useState<{ url: string; revision: number } | null>(
		null,
	);
	const [failed, setFailed] = useState(false);
	const [attempt, setAttempt] = useState(0);

	// A picture the server is too busy to draw (429) or that failed is asked for again twice, after a short wait,
	// before the screen says so; a picture that is no longer wanted is dropped so its request is cancelled.
	// biome-ignore lint/correctness/useExhaustiveDependencies: `attempt` restarts the load after a retry.
	useEffect(() => {
		let live = true;
		let tries = 0;
		let timer: number | undefined;
		const loader = new Image();
		const load = (): void => {
			loader.src = wanted;
		};
		loader.onload = () => {
			if (!live) return;
			setShown({ url: wanted, revision });
			setFailed(false);
		};
		loader.onerror = () => {
			if (!live) return;
			if (tries < RETRIES) {
				tries += 1;
				timer = window.setTimeout(load, RETRY_MS * tries);
				return;
			}
			setFailed(true);
		};
		load();
		return () => {
			live = false;
			window.clearTimeout(timer);
			loader.onload = null;
			loader.onerror = null;
			loader.src = "";
		};
	}, [wanted, attempt]);

	return (
		<>
			{shown === null ? (
				<div
					data-testid="image-frame-loading"
					className="size-full animate-pulse bg-muted"
					role="img"
					aria-label={t("ui.image.canvas.loading")}
				/>
			) : (
				<img
					data-testid="image-frame"
					data-revision={shown.revision}
					src={shown.url}
					width={width * zoom}
					height={height * zoom}
					alt={t("ui.image.canvas.picture")}
					draggable={false}
					className="block size-full select-none"
				/>
			)}
			{failed && (
				<div
					role="alert"
					className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-2 bg-destructive px-2 py-1 text-xs text-white"
				>
					<span>{t("ui.image.canvas.failed")}</span>
					<button
						type="button"
						className="rounded underline"
						onClick={() => setAttempt((n) => n + 1)}
					>
						{t("ui.image.canvas.retry")}
					</button>
				</div>
			)}
		</>
	);
}
