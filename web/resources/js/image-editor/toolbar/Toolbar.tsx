import { Maximize, Minus, Plus, ScanSearch } from "lucide-react";
import { type KeyboardEvent, type ReactNode, useEffect, useRef } from "react";

import { useT } from "@/lib/i18n";
import { ZOOM_STEP } from "../canvas/viewport";
import { useImage, useImageStore } from "../state/context";
import ToolButton from "./ToolButton";

// The items the arrow keys walk: the enabled buttons that carry data-tool.
function items(root: HTMLElement): HTMLElement[] {
	return Array.from(
		root.querySelectorAll<HTMLElement>("[data-tool]:not([disabled])"),
	);
}

// The toolbar of design system 4.3.1: one tab stop, Left and Right between the buttons, Home and End to the
// ends. The tab stop is kept by hand because a button becomes disabled and enabled as the selection changes.
export function Toolbar({
	label,
	children,
}: {
	label: string;
	children: ReactNode;
}): ReactNode {
	const ref = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const root = ref.current;
		if (root === null) return;
		const settle = (): void => {
			const enabled = items(root);
			const current = enabled.find((el) => el.tabIndex === 0);
			for (const el of root.querySelectorAll<HTMLElement>("[data-tool]")) {
				if (el !== (current ?? enabled[0])) el.tabIndex = -1;
			}
			const stop = current ?? enabled[0];
			if (stop !== undefined) stop.tabIndex = 0;
		};
		settle();
		const observer = new MutationObserver(settle);
		observer.observe(root, {
			subtree: true,
			childList: true,
			attributes: true,
			attributeFilter: ["disabled"],
		});
		return () => observer.disconnect();
	}, []);

	const onKeyDown = (event: KeyboardEvent<HTMLDivElement>): void => {
		const root = ref.current;
		if (root === null || event.altKey || event.ctrlKey || event.metaKey) return;
		const list = items(root);
		const at = list.indexOf(document.activeElement as HTMLElement);
		if (at < 0) return;
		let next: number;
		switch (event.key) {
			case "ArrowRight":
				next = (at + 1) % list.length;
				break;
			case "ArrowLeft":
				next = (at - 1 + list.length) % list.length;
				break;
			case "Home":
				next = 0;
				break;
			case "End":
				next = list.length - 1;
				break;
			default:
				return;
		}
		event.preventDefault();
		for (const el of list) el.tabIndex = -1;
		const target = list[next];
		if (target !== undefined) {
			target.tabIndex = 0;
			target.focus();
		}
	};

	return (
		<div
			ref={ref}
			role="toolbar"
			aria-label={label}
			data-testid="image-toolbar"
			onKeyDown={onKeyDown}
			className="flex flex-wrap items-center gap-1 border-b bg-background px-2 py-1"
		>
			{children}
		</div>
	);
}

export function ToolGroup({ children }: { children: ReactNode }): ReactNode {
	return (
		<>
			<div className="flex items-center gap-0.5">{children}</div>
			<span aria-hidden="true" className="mx-1 h-5 w-px bg-border" />
		</>
	);
}

// The buttons that change the view: zoom out, zoom in, fit and 100%.
export function ViewTools(): ReactNode {
	const t = useT();
	const store = useImageStore();
	const doc = useImage((s) => s.doc);
	const off = doc === null;

	return (
		<ToolGroup>
			<ToolButton
				icon={Minus}
				label={t("ui.image.view.zoom_out")}
				shortcut="-"
				disabled={off}
				onClick={() => store.getState().zoomBy(1 / ZOOM_STEP)}
				testId="image-zoom-out"
			/>
			<ToolButton
				icon={Plus}
				label={t("ui.image.view.zoom_in")}
				shortcut="+"
				disabled={off}
				onClick={() => store.getState().zoomBy(ZOOM_STEP)}
				testId="image-zoom-in"
			/>
			<ToolButton
				icon={Maximize}
				label={t("ui.image.view.fit")}
				shortcut="0"
				disabled={off}
				onClick={() => store.getState().fitView()}
				testId="image-zoom-fit"
			/>
			<ToolButton
				icon={ScanSearch}
				label={t("ui.image.view.actual")}
				shortcut="1"
				disabled={off}
				onClick={() => store.getState().actualSize()}
				testId="image-zoom-100"
			/>
		</ToolGroup>
	);
}
