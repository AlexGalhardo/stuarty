import type { ReactNode } from "react";

import { useDocumentState } from "../../state/EditorContext";

// The two edges of an item that carry a transition (SDD-012 section 7.4): a
// strip as long as the transition, placed by the zoom variable of the timeline
// like the item. It draws what the read model stores and never the picture of
// the transition, which only the plan window shows.
export default function TransitionMarks({ id }: { id: string }): ReactNode {
	const inLength = useDocumentState(
		(s) => s.document?.items[id]?.transition_in?.duration,
	);
	const outLength = useDocumentState(
		(s) => s.document?.items[id]?.transition_out?.duration,
	);
	return (
		<>
			{inLength !== undefined && (
				<span
					aria-hidden="true"
					data-transition-mark="in"
					className="pointer-events-none absolute inset-y-0 left-0 bg-gradient-to-r from-foreground/40 to-transparent"
					style={{
						["--td" as string]: inLength,
						width: "calc(var(--td) * var(--ppf) * 1px)",
					}}
				/>
			)}
			{outLength !== undefined && (
				<span
					aria-hidden="true"
					data-transition-mark="out"
					className="pointer-events-none absolute inset-y-0 right-0 bg-gradient-to-l from-foreground/40 to-transparent"
					style={{
						["--td" as string]: outLength,
						width: "calc(var(--td) * var(--ppf) * 1px)",
					}}
				/>
			)}
		</>
	);
}
