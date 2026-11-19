import {
	type KeyboardEvent,
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import { type Translate, useT } from "@/lib/i18n";
import { useImage, useImageStore } from "../state/context";
import { type LayerBox, layerBoxes } from "./boxes";
import Frame from "./Frame";
import { nudgeOps } from "./nudge";
import Overlay from "./Overlay";
import { useGestures } from "./useGestures";
import { effectiveZoom, origin, ZOOM_STEP } from "./viewport";

// The stage of the image editor (SDD-012 section 24.4): the picture of the document at the zoom and the pan of
// the viewport, and the overlay of the selection. Zoom and pan are display state and are never sent.
export default function Canvas(): ReactNode {
	const t = useT();
	const store = useImageStore();
	const doc = useImage((s) => s.doc);
	const view = useImage((s) => s.view);
	const stage = useImage((s) => s.stage);
	const stageRef = useRef<HTMLDivElement>(null);
	const space = useRef(false);
	const panning = useRef<{
		x: number;
		y: number;
		panX: number;
		panY: number;
	} | null>(null);
	const [grabbing, setGrabbing] = useState(false);
	const selection = useImage((s) => s.selection);
	const changed = useImage((s) => s.changedByOthers);
	const gestures = useGestures(stageRef);
	const boxes = useMemo(
		() => (doc === null ? [] : layerBoxes(doc.layers)),
		[doc],
	);

	useEffect(() => {
		const el = stageRef.current;
		if (el === null) return;
		const measure = (): void =>
			store.getState().setStage({ w: el.clientWidth, h: el.clientHeight });
		measure();
		const observer = new ResizeObserver(measure);
		observer.observe(el);
		return () => observer.disconnect();
	}, [store]);

	// The wheel must be able to cancel the scroll of the page, which a React handler cannot (it is passive).
	useEffect(() => {
		const el = stageRef.current;
		if (el === null) return;
		const onWheel = (event: WheelEvent): void => {
			event.preventDefault();
			const state = store.getState();
			const rect = el.getBoundingClientRect();
			if (event.ctrlKey || event.metaKey) {
				const factor = event.deltaY < 0 ? ZOOM_STEP : 1 / ZOOM_STEP;
				state.zoomBy(factor, {
					x: event.clientX - rect.left,
					y: event.clientY - rect.top,
				});
				return;
			}
			const now = state.view;
			state.setView({
				fit: now.fit,
				panX: now.panX - event.deltaX,
				panY: now.panY - event.deltaY,
			});
		};
		el.addEventListener("wheel", onWheel, { passive: false });
		return () => el.removeEventListener("wheel", onWheel);
	}, [store]);

	const announce = useAnnouncement(t, boxes, selection);
	const size = doc === null ? { w: 0, h: 0 } : { w: doc.width, h: doc.height };
	const zoom = effectiveZoom(view, stage, size);
	const at = origin(view, stage, size, zoom);

	const onPointerDown = useCallback(
		(event: ReactPointerEvent<HTMLDivElement>) => {
			stageRef.current?.focus();
			if (event.button === 1 || (event.button === 0 && space.current)) {
				event.preventDefault();
				const { view: v } = store.getState();
				panning.current = {
					x: event.clientX,
					y: event.clientY,
					panX: v.panX,
					panY: v.panY,
				};
				setGrabbing(true);
				event.currentTarget.setPointerCapture(event.pointerId);
				return;
			}
			gestures.onPointerDown(event);
		},
		[store, gestures.onPointerDown],
	);

	const onPointerMove = useCallback(
		(event: ReactPointerEvent<HTMLDivElement>) => {
			const start = panning.current;
			if (start === null) {
				gestures.onPointerMove(event);
				return;
			}
			store.getState().setView({
				panX: start.panX + event.clientX - start.x,
				panY: start.panY + event.clientY - start.y,
			});
		},
		[store, gestures.onPointerMove],
	);

	const endPointer = (event: ReactPointerEvent<HTMLDivElement>): void => {
		if (panning.current !== null) {
			panning.current = null;
			setGrabbing(false);
			return;
		}
		gestures.onPointerUp(event);
	};

	const onKeyDown = (event: KeyboardEvent<HTMLDivElement>): void => {
		if (event.key === " ") {
			space.current = true;
			event.preventDefault();
			return;
		}
		if (event.key === "Escape") {
			gestures.cancel();
			return;
		}
		if (event.ctrlKey || event.metaKey || event.altKey) return;
		const state = store.getState();
		const nudge = NUDGE[event.key];
		if (nudge !== undefined) {
			event.preventDefault();
			const step = event.shiftKey ? 10 : 1;
			const ops = nudgeOps(state, nudge[0] * step, nudge[1] * step);
			if (ops.length > 0) void state.send(ops);
			return;
		}
		if (event.key === "Delete" || event.key === "Backspace") {
			event.preventDefault();
			const ops = state.selection.map((id) => ({
				op: "layer.remove",
				args: { layer_id: id },
			}));
			if (ops.length > 0) void state.send(ops);
			return;
		}
		switch (event.key) {
			case "+":
			case "=":
				state.zoomBy(ZOOM_STEP);
				break;
			case "-":
				state.zoomBy(1 / ZOOM_STEP);
				break;
			case "0":
				state.fitView();
				break;
			case "1":
				state.actualSize();
				break;
			default:
				return;
		}
		event.preventDefault();
	};

	return (
		// biome-ignore lint/a11y/useSemanticElements: the stage is a pointer surface with its own keys; a fieldset or a button would be wrong.
		<div
			ref={stageRef}
			role="group"
			aria-label={t("ui.image.canvas.label")}
			data-testid="image-stage"
			data-zoom={zoom.toFixed(4)}
			// biome-ignore lint/a11y/noNoninteractiveTabindex: the stage is the one focus stop of the canvas; its keys are the zoom, the nudges and Delete.
			tabIndex={0}
			onPointerDown={onPointerDown}
			onPointerMove={onPointerMove}
			onPointerUp={endPointer}
			onPointerCancel={endPointer}
			onKeyDown={onKeyDown}
			onKeyUp={(event) => {
				if (event.key === " ") space.current = false;
			}}
			className="relative size-full touch-none overflow-hidden bg-ed-preview-surround outline-offset-[-2px]"
			style={{ cursor: grabbing ? "grabbing" : undefined }}
		>
			{doc !== null && stage.w > 0 && (
				<div
					data-testid="image-picture"
					className="absolute shadow-lg [background:repeating-conic-gradient(#808080_0_25%,#c0c0c0_0_50%)_50%/16px_16px]"
					style={{
						left: at.x,
						top: at.y,
						width: doc.width * zoom,
						height: doc.height * zoom,
					}}
				>
					<Frame
						imageId={doc.image_id}
						revision={doc.revision}
						width={doc.width}
						height={doc.height}
						zoom={zoom}
					/>
				</div>
			)}
			{doc !== null && stage.w > 0 && (
				<Overlay
					boxes={boxes}
					selection={selection}
					changed={changed}
					drag={gestures.drag}
					zoom={zoom}
					origin={at}
					canvas={{ w: doc.width, h: doc.height }}
				/>
			)}
			<div role="status" aria-live="polite" className="sr-only">
				{announce}
			</div>
		</div>
	);
}

const NUDGE: Record<string, [number, number]> = {
	ArrowLeft: [-1, 0],
	ArrowRight: [1, 0],
	ArrowUp: [0, -1],
	ArrowDown: [0, 1],
};

// The words that tell a screen reader what is selected (SDD-012 section 24.4).
function useAnnouncement(
	t: Translate,
	boxes: readonly LayerBox[],
	selection: readonly string[],
): string {
	if (selection.length === 0) return t("ui.image.canvas.none_selected");
	if (selection.length > 1) {
		return t("ui.image.canvas.many_selected", { count: selection.length });
	}
	const box = boxes.find((b) => b.id === selection[0]);
	if (box === undefined) return "";
	return t("ui.image.canvas.selected", {
		name: box.name,
		kind: t(`ui.image.kind.${box.kind}`),
		x: box.x,
		y: box.y,
		w: box.w,
		h: box.h,
	});
}
