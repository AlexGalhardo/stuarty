import { useEffect, useState } from "react";

import type { ClipZoom } from "../api/types";
import { useDocumentState, useRuntime } from "../state/EditorContext";

// The zooms of the clips that play the given assets, read from the service (`media_inspect`, section `zooms`,
// SDD-025 section 17.8) and read again whenever the project moves to a new revision. The editor finds no zoom
// itself: the core finds them in the scale keyframes and the service gives them their times.
export function useZooms(assetIds: readonly string[]): ClipZoom[] {
	const runtime = useRuntime();
	const revision = useDocumentState((s) => s.revision);
	const [zooms, setZooms] = useState<ClipZoom[]>([]);
	const key = assetIds.join(",");
	// biome-ignore lint/correctness/useExhaustiveDependencies: `key` stands for the list of ids; revision is the trigger
	useEffect(() => {
		let current = true;
		const ids = key === "" ? [] : key.split(",");
		Promise.all(
			ids.map((id) =>
				runtime.client.inspectMedia(runtime.projectId, id, {
					sections: ["zooms"],
				}),
			),
		)
			.then((results) => {
				if (current) setZooms(results.flatMap((r) => r.zooms ?? []));
			})
			.catch(() => {
				if (current) setZooms([]);
			});
		return () => {
			current = false;
		};
	}, [runtime, key, revision]);
	return zooms;
}

// The assets of the clips that hold scale keyframes, as a stable list: a clip with none has no zoom to ask about.
export function useAnimatedAssets(): string[] {
	const key = useDocumentState((s) => {
		const ids = new Set<string>();
		for (const item of Object.values(s.document?.items ?? {})) {
			if (
				item.type === "clip" &&
				item.asset !== undefined &&
				(item.keyframes?.scale?.length ?? 0) > 1
			) {
				ids.add(item.asset);
			}
		}
		return [...ids].sort().join(",");
	});
	return key === "" ? [] : key.split(",");
}
