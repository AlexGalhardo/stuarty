import type { ReactNode } from "react";

import { useStore } from "../state/createStore";
import { useRuntime } from "../state/EditorContext";
import { frameToContentX } from "./geometry";

// The line drawn at the frame a gesture snapped to: it reads ghostStore only, so
// the pointer-rate updates re-render nothing else (SDD-012 section 19.2).
export default function SnapLine({ ppf }: { ppf: number }): ReactNode {
	const runtime = useRuntime();
	const frame = useStore(runtime.ghosts, (s) => s.snapLine);
	if (frame === null) return null;
	return (
		<div
			aria-hidden="true"
			data-testid="snap-line"
			data-frame={frame}
			className="tl-snapline pointer-events-none absolute top-0 bottom-0 left-0 w-0 border-l-2 border-ed-snap"
			style={{ transform: `translateX(${frameToContentX(frame, ppf)}px)` }}
		/>
	);
}
