import { useEffect, useState } from "react";

import { useRuntime } from "./EditorContext";

export interface TimelineName {
	id: string;
	title: string;
}

// The active timelines of the production the editor is open on, by id, read
// once and again when the page gets the focus back (the names change in
// another tab). A composition item shows the name of the timeline it uses, and
// the breadcrumb of a nested timeline the name of the one that opened it.
export function useProductionTimelines(): ReadonlyMap<string, string> {
	const runtime = useRuntime();
	const [names, setNames] = useState<ReadonlyMap<string, string>>(new Map());
	useEffect(() => {
		let live = true;
		const load = (): void => {
			runtime.client
				.describeProject(runtime.projectId)
				.then((result) => {
					if (!live) return;
					const next = new Map<string, string>();
					for (const item of result.items) {
						if (
							item.kind === "timeline" &&
							item.state === "active" &&
							item.timeline_id !== undefined
						)
							next.set(item.timeline_id, item.title);
					}
					setNames(next);
				})
				.catch(() => undefined);
		};
		load();
		window.addEventListener("focus", load);
		return () => {
			live = false;
			window.removeEventListener("focus", load);
		};
	}, [runtime]);
	return names;
}
