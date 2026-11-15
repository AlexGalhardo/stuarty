import { type ReactNode, useRef } from "react";

import { useT } from "@/lib/i18n";
import type { CanvasPort, LayerGeometry } from "../types";

// The inline editor of a text item (SDD-012 sections 8.2, 11.6 and 12.3): a
// `<textarea>` over the item's box. Leaving it or Ctrl+Enter sends the content
// with `text.set`; Esc discards. It never draws the text: the picture comes from
// the bitmap of the plan once the new revision arrives. Without a plan layer
// (the playhead is outside the item) it sits in the middle of the canvas.
export default function InlineTextEditor({
	itemId,
	stored,
	geometry,
	scale,
	size,
	port,
	onClose,
}: {
	itemId: string;
	stored: string;
	geometry: LayerGeometry | null;
	// Screen pixels per project pixel.
	scale: number;
	// The canvas box in screen pixels, for the place without a layer.
	size: { w: number; h: number };
	port: CanvasPort;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const finished = useRef(false);
	const finish = (value: string | null): void => {
		if (finished.current) return;
		finished.current = true;
		onClose();
		if (value !== null && value !== "" && value !== stored) {
			port.commitText(itemId, value);
		}
	};
	const box =
		geometry === null
			? {
					left: size.w * 0.1,
					top: size.h * 0.4,
					width: size.w * 0.8,
					height: size.h * 0.2,
					transform: undefined,
				}
			: {
					left: (geometry.cx - geometry.w / 2) * scale,
					top: (geometry.cy - geometry.h / 2) * scale,
					width: geometry.w * scale,
					height: geometry.h * scale,
					transform: `rotate(${geometry.rotation}deg)`,
				};
	return (
		<textarea
			data-testid="inline-text-editor"
			// biome-ignore lint/a11y/noAutofocus: the editor opens on a double click and the person types at once.
			autoFocus
			aria-label={t("ui.editor.text.editor_label")}
			defaultValue={stored}
			maxLength={2000}
			className="absolute z-10 resize-none border-2 border-ed-handle bg-black/60 p-1 text-center text-white outline-none"
			style={{
				...box,
				minWidth: 120,
				minHeight: 40,
			}}
			onFocus={(event) => event.currentTarget.select()}
			onPointerDown={(event) => event.stopPropagation()}
			onClick={(event) => event.stopPropagation()}
			onDoubleClick={(event) => event.stopPropagation()}
			onKeyDown={(event) => {
				event.stopPropagation();
				if (event.key === "Escape") finish(null);
				else if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
					event.preventDefault();
					finish(event.currentTarget.value);
				}
			}}
			onBlur={(event) => finish(event.currentTarget.value)}
		/>
	);
}
