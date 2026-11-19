import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import type { LayerBox } from "./boxes";
import { type Box, HANDLES, type Handle } from "./gestures";
import type { Drag } from "./useGestures";
import type { Point } from "./viewport";

// The place of each handle on the box, as a fraction of its width and height.
const AT: Record<Handle, [number, number]> = {
	nw: [0, 0],
	n: [0.5, 0],
	ne: [1, 0],
	e: [1, 0.5],
	se: [1, 1],
	s: [0.5, 1],
	sw: [0, 1],
	w: [0, 0.5],
};

const CURSOR: Record<Handle, string> = {
	nw: "nwse-resize",
	se: "nwse-resize",
	ne: "nesw-resize",
	sw: "nesw-resize",
	n: "ns-resize",
	s: "ns-resize",
	e: "ew-resize",
	w: "ew-resize",
};

interface Props {
	boxes: readonly LayerBox[];
	selection: readonly string[];
	changed: readonly string[];
	drag: Drag | null;
	zoom: number;
	origin: Point;
	canvas: { w: number; h: number };
}

// The outlines drawn over the picture: the selection with its handles, the guides of the snap and, in the
// colour of the agents, the layers another actor changed. The boxes are those of the read model, or the
// transient ones of the drag in progress; nothing here is a result.
export default function Overlay({
	boxes,
	selection,
	changed,
	drag,
	zoom,
	origin,
	canvas,
}: Props): ReactNode {
	const t = useT();
	const byId = new Map(boxes.map((b) => [b.id, b]));
	const place = (b: Box): React.CSSProperties => ({
		position: "absolute",
		left: origin.x + b.x * zoom,
		top: origin.y + b.y * zoom,
		width: b.w * zoom,
		height: b.h * zoom,
		transform: b.rotation === 0 ? undefined : `rotate(${b.rotation}deg)`,
	});
	const boxOf = (id: string): Box | undefined => {
		const moved = drag?.boxes.get(id);
		if (moved !== undefined) return moved;
		const b = byId.get(id);
		return b === undefined
			? undefined
			: { x: b.x, y: b.y, w: b.w, h: b.h, rotation: b.rotation };
	};
	const single =
		selection.length === 1 ? byId.get(selection[0] ?? "") : undefined;
	const handles =
		single !== undefined && single.kind !== "group" && !single.locked;

	return (
		<>
			{changed.map((id) => {
				const b = boxOf(id);
				if (b === undefined || b.w === 0 || !byId.get(id)?.visible) return null;
				return (
					<div
						key={`changed-${id}`}
						data-testid="image-changed"
						data-layer={id}
						aria-hidden="true"
						className="pointer-events-none border-2 border-dashed border-agent"
						style={place(b)}
					/>
				);
			})}
			{selection.map((id) => {
				const b = boxOf(id);
				if (b === undefined) return null;
				const own = byId.get(id);
				return (
					<div
						key={id}
						data-testid="image-selection"
						data-layer={id}
						aria-hidden="true"
						className="pointer-events-none border-2 border-ed-handle"
						style={place(b)}
					>
						<span className="absolute left-0 top-full mt-1 whitespace-nowrap rounded bg-ed-handle px-1 text-[10px] text-background tabular-nums">
							{own?.locked
								? t("ui.image.canvas.locked")
								: t("ui.image.editor.size", { w: b.w, h: b.h })}
						</span>
						{handles && single?.id === id && (
							<>
								{HANDLES.map((h) => (
									<span
										key={h}
										data-handle={h}
										className="pointer-events-auto absolute flex size-6 -translate-x-1/2 -translate-y-1/2 items-center justify-center"
										style={{
											left: `${AT[h][0] * 100}%`,
											top: `${AT[h][1] * 100}%`,
											cursor: CURSOR[h],
										}}
									>
										<span
											data-handle={h}
											className="size-2.5 border-2 border-ed-handle bg-background"
										/>
									</span>
								))}
								<span
									data-handle="rotate"
									className="pointer-events-auto absolute left-1/2 flex size-6 -translate-x-1/2 -translate-y-full cursor-grab items-center justify-center"
									style={{ top: -12 }}
								>
									<span
										data-handle="rotate"
										className="size-2.5 rounded-full border-2 border-ed-handle bg-background"
									/>
								</span>
							</>
						)}
					</div>
				);
			})}
			{drag?.guideX != null && (
				<div
					data-testid="image-guide-x"
					aria-hidden="true"
					className="pointer-events-none absolute w-px border-l border-dashed border-ed-snap"
					style={{
						left: origin.x + drag.guideX * zoom,
						top: origin.y,
						height: canvas.h * zoom,
					}}
				/>
			)}
			{drag?.guideY != null && (
				<div
					data-testid="image-guide-y"
					aria-hidden="true"
					className="pointer-events-none absolute h-px border-t border-dashed border-ed-snap"
					style={{
						top: origin.y + drag.guideY * zoom,
						left: origin.x,
						width: canvas.w * zoom,
					}}
				/>
			)}
		</>
	);
}
