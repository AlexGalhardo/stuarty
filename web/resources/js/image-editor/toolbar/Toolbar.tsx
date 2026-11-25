import {
	createContext,
	type KeyboardEvent,
	type ReactNode,
	useContext,
	useEffect,
	useRef,
} from "react";

import { cn } from "@/lib/cn";

// The items the arrow keys walk: the enabled buttons that carry data-tool.
function items(root: HTMLElement): HTMLElement[] {
	return Array.from(
		root.querySelectorAll<HTMLElement>("[data-tool]:not([disabled])"),
	);
}

type Orientation = "horizontal" | "vertical";

const OrientationContext = createContext<Orientation>("horizontal");

// The toolbar of design system 4.3.1: one tab stop, the arrows of its axis between the buttons, Home and End to
// the ends. The tab stop is kept by hand because a button becomes disabled and enabled as the selection changes.
// The tool rail of the final design is a vertical one (Up and Down); the strip under the stage is horizontal.
export function Toolbar({
	label,
	children,
	orientation = "horizontal",
	testId = "image-toolbar",
	className,
}: {
	label: string;
	children: ReactNode;
	orientation?: Orientation;
	testId?: string;
	className?: string;
}): ReactNode {
	const vertical = orientation === "vertical";
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
			case vertical ? "ArrowDown" : "ArrowRight":
				next = (at + 1) % list.length;
				break;
			case vertical ? "ArrowUp" : "ArrowLeft":
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
		<OrientationContext.Provider value={orientation}>
			<div
				ref={ref}
				role="toolbar"
				aria-label={label}
				aria-orientation={orientation}
				data-testid={testId}
				onKeyDown={onKeyDown}
				className={cn(
					"flex items-center gap-1 bg-ed-panel",
					vertical
						? "w-11 shrink-0 flex-col overflow-y-auto border-r border-border px-1 py-2"
						: "px-2 py-1",
					className,
				)}
			>
				{children}
			</div>
		</OrientationContext.Provider>
	);
}

// A named run of buttons of the toolbar: the name is what a reader says on entering it, and the rule after
// it is what separates it from the next run.
export function ToolGroup({
	label,
	children,
}: {
	label: string;
	children: ReactNode;
}): ReactNode {
	const vertical = useContext(OrientationContext) === "vertical";
	return (
		<>
			<fieldset
				aria-label={label}
				className={cn(
					"m-0 flex min-w-0 items-center gap-0.5 border-0 p-0",
					vertical && "flex-col",
				)}
			>
				{children}
			</fieldset>
			<span
				aria-hidden="true"
				className={cn(
					"bg-border last:hidden",
					vertical ? "my-1 h-px w-5" : "mx-1 h-5 w-px",
				)}
			/>
		</>
	);
}
