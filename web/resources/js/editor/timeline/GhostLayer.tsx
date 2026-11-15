import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { useStore } from "../state/createStore";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { selectOrderedTrackIds, selectTrack } from "../state/selectors";

// The ghosts of a drag (SDD-012 sections 7.1 and 8.1): outlines at the requested
// place, over the rows, never touching the read model. Only this component and
// the snap line follow the pointer, so the items do not re-render during a drag.
export default function GhostLayer(): ReactNode {
	const runtime = useRuntime();
	const ghosts = useStore(runtime.ghosts, (s) => s.ghosts);
	const rowPx = useUiState((s) => s.rowPx);
	const tops = useDocumentState((s) => {
		const out: Record<string, { top: number; height: number }> = {};
		let top = 0;
		for (const id of selectOrderedTrackIds(s)) {
			const track = selectTrack(s, id);
			const height = track?.kind === "audio" ? rowPx.audio : rowPx.visual;
			out[id] = { top, height };
			top += height;
		}
		return JSON.stringify(out);
	});
	const entries = Object.entries(ghosts);
	if (entries.length === 0) return null;
	const rows = JSON.parse(tops) as Record<
		string,
		{ top: number; height: number }
	>;
	return (
		<div
			aria-hidden="true"
			data-testid="ghost-layer"
			className="tl-ghost-layer pointer-events-none absolute inset-0"
		>
			{entries.map(([id, ghost]) => (
				<div
					key={id}
					data-ghost-id={id}
					data-ghost-track={ghost.track}
					data-pending={ghost.pending}
					className={cn(
						"absolute rounded-md border-2 bg-ed-ghost",
						ghost.pending
							? "border-dashed border-ed-handle"
							: "border-ed-handle",
					)}
					style={{
						top: (rows[ghost.track]?.top ?? 0) + 4,
						height: (rows[ghost.track]?.height ?? 64) - 8,
						left: `calc(${ghost.start} * var(--ppf) * 1px)`,
						width: `calc(${ghost.duration} * var(--ppf) * 1px)`,
					}}
				/>
			))}
		</div>
	);
}
