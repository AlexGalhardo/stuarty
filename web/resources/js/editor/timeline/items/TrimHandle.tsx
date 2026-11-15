import type { ReactNode } from "react";

import { useActions } from "../../commands/useActions";
import { useRuntime } from "../../state/EditorContext";
import { beginTrim } from "../gestures/trim";

// A trim handle at one edge of an item (SDD-012 section 8.2): an 8 px bar to
// see, a 24 px zone to grab. It is for the pointer; the keyboard trims with [
// and ] to the playhead.
export default function TrimHandle({
	itemId,
	edge,
}: {
	itemId: string;
	edge: "start" | "end";
}): ReactNode {
	const runtime = useRuntime();
	const actions = useActions();
	return (
		<span
			aria-hidden="true"
			data-trim-handle={edge}
			className={
				edge === "start"
					? "absolute inset-y-0 left-0 flex w-6 max-w-1/4 cursor-ew-resize items-center justify-start"
					: "absolute inset-y-0 right-0 flex w-6 max-w-1/4 cursor-ew-resize items-center justify-end"
			}
			onPointerDown={(event) => {
				if (event.button !== 0) return;
				// The item must not start a move under the handle.
				event.stopPropagation();
				beginTrim({ runtime, actions, event, itemId, edge });
			}}
		>
			<span className="h-1/2 w-full max-w-2 rounded-sm bg-ed-handle opacity-0 group-hover:opacity-100 group-aria-selected:opacity-100" />
		</span>
	);
}
