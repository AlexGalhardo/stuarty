import type { ReactNode } from "react";

import { useDocumentState } from "../../state/EditorContext";

// The markers of an item as thin ticks (SDD-012 section 7.4, BETA). The frames
// come from the core (`audio.mark_beats` writes them); the browser only draws
// them at `frame * --ppf`. Decoration: they have no effect on the picture or
// the sound, so they take no pointer events.
export default function MarkerTicks({ id }: { id: string }): ReactNode {
	const markers = useDocumentState((s) => s.document?.items[id]?.markers);
	if (markers === undefined || markers.length === 0) return null;
	return (
		<span
			aria-hidden="true"
			data-marker-ticks=""
			className="pointer-events-none absolute inset-0 overflow-hidden"
		>
			{markers.map((marker) => (
				<span
					key={marker.frame}
					data-marker-frame={marker.frame}
					title={marker.label}
					className="absolute inset-y-0 w-px bg-foreground/60"
					style={{
						["--mf" as string]: marker.frame,
						left: "calc(var(--mf) * var(--ppf) * 1px)",
					}}
				/>
			))}
		</span>
	);
}
