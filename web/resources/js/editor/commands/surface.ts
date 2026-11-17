import { useEffect } from "react";

// The surfaces whose state lives in the component that draws them (the export
// dialog, the cheat sheet, the Add caption dialog). The menu asks for one with
// `requestSurface`; the component that owns the state answers it, so a menu item
// and the control of the page open the same dialog.
export type SurfaceRequest = "export" | "cheat_sheet" | "add_caption";

const SURFACE_EVENT = "vukem-surface";

export function requestSurface(kind: SurfaceRequest): void {
	window.dispatchEvent(
		new CustomEvent<SurfaceRequest>(SURFACE_EVENT, { detail: kind }),
	);
}

export function useSurfaceRequest(
	kind: SurfaceRequest,
	handler: () => void,
): void {
	useEffect(() => {
		const onRequest = (event: Event): void => {
			if ((event as CustomEvent<SurfaceRequest>).detail === kind) handler();
		};
		window.addEventListener(SURFACE_EVENT, onRequest);
		return () => window.removeEventListener(SURFACE_EVENT, onRequest);
	}, [kind, handler]);
}
