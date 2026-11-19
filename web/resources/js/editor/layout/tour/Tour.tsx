import {
	type CSSProperties,
	type KeyboardEvent as ReactKeyboardEvent,
	type ReactNode,
	useEffect,
	useId,
	useRef,
	useState,
	useSyncExternalStore,
} from "react";

import { Button } from "@/components/ui/button";
import {
	canDropPaths,
	canPickFiles,
	getDesktopBridge,
	subscribeDesktopBridge,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import { IS_MAC } from "../../commands/keymap";
import { keyLabel, useKeymap } from "../../commands/keymapStore";
import {
	endTour,
	goToTourStep,
	rememberTour,
	useTour,
} from "../../state/tourStore";
import { TOUR_STEPS, type TourPlacement } from "./steps";

interface Box {
	left: number;
	top: number;
	width: number;
	height: number;
}

// Below this width the card sits at the bottom centre instead of beside the
// region (docs/design-system.md 4.3.18).
const NARROW_PX = 1280;
const GAP_PX = 12;

function cardStyle(box: Box | null, placement: TourPlacement): CSSProperties {
	if (box === null || window.innerWidth < NARROW_PX) {
		return { left: "50%", bottom: 16, transform: "translateX(-50%)" };
	}
	const right = box.left + box.width;
	const bottom = box.top + box.height;
	switch (placement) {
		case "right":
			return { left: right + GAP_PX, top: box.top + GAP_PX };
		case "inside":
			return { left: box.left + GAP_PX, top: box.top + GAP_PX };
		case "above":
			return {
				left: box.left + GAP_PX,
				bottom: window.innerHeight - box.top + GAP_PX,
			};
		case "left":
			return {
				right: window.innerWidth - box.left + GAP_PX,
				top: box.top + GAP_PX,
			};
		case "below":
			return { right: window.innerWidth - right, top: bottom + GAP_PX };
	}
}

// The first-run tour (SDD-012 section 18.5): five steps, each outlining its
// region and showing a card with a sentence, Back, Next and Skip. The card is
// not modal, takes the focus when a step opens and gives it back when the tour
// ends, and Esc skips it. Showing it is what remembers it (`vv.shortcutsSeen`).
export default function Tour(): ReactNode {
	const t = useT();
	const { open, step } = useTour();
	const keymap = useKeymap();
	const titleId = useId();
	const bodyId = useId();
	const card = useRef<HTMLElement>(null);
	const returnTo = useRef<HTMLElement | null>(null);
	const [box, setBox] = useState<Box | null>(null);
	// In the desktop window the library also has "Choose files" and drops.
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const desktop = canPickFiles(bridge) || canDropPaths(bridge);
	const index = Math.min(step, TOUR_STEPS.length - 1);
	const current = TOUR_STEPS[index];

	useEffect(() => {
		if (open) rememberTour();
	}, [open]);

	// The element that had the focus gets it back when the tour ends.
	useEffect(() => {
		if (!open) return;
		const active = document.activeElement;
		returnTo.current =
			active instanceof HTMLElement && active !== document.body ? active : null;
		return () => {
			returnTo.current?.focus();
			returnTo.current = null;
		};
	}, [open]);

	// The outline follows the region of the step.
	const target = current?.target;
	useEffect(() => {
		if (!open || target === undefined) return;
		const measure = (): void => {
			const rect = document.querySelector(target)?.getBoundingClientRect();
			setBox(
				rect === undefined || rect.width === 0 || rect.height === 0
					? null
					: {
							left: rect.left,
							top: rect.top,
							width: rect.width,
							height: rect.height,
						},
			);
		};
		measure();
		window.addEventListener("resize", measure);
		const element = document.querySelector(target);
		const observer = new ResizeObserver(measure);
		if (element !== null) observer.observe(element);
		return () => {
			window.removeEventListener("resize", measure);
			observer.disconnect();
		};
	}, [open, target]);

	// The "Next" button takes the focus when a step opens.
	// biome-ignore lint/correctness/useExhaustiveDependencies: `step` is the trigger: a new step moves the focus to Next again.
	useEffect(() => {
		if (!open) return;
		// A frame later, so a menu that started the tour has given the focus back
		// to its button first and does not take it from the card.
		const frame = requestAnimationFrame(() =>
			card.current?.querySelector<HTMLElement>("[data-tour-next]")?.focus(),
		);
		return () => cancelAnimationFrame(frame);
	}, [open, step]);

	if (!open || current === undefined) return null;

	const key = (row: string): string => keyLabel(row, keymap, IS_MAC) ?? "";
	const params = {
		import: key("import"),
		play: key("play_pause"),
		split: key("split"),
		undo: key("undo"),
		export: key("export"),
		shortcuts: key("cheat_sheet"),
		palette: key("palette"),
	};
	const last = index === TOUR_STEPS.length - 1;
	const bodyKey =
		current.id === "library" && desktop
			? "ui.editor.tour.steps.library_desktop"
			: `ui.editor.tour.steps.${current.id}.body`;

	const onKeyDown = (event: ReactKeyboardEvent<HTMLElement>): void => {
		if (event.key !== "Escape") return;
		event.preventDefault();
		event.stopPropagation();
		endTour();
	};

	return (
		<>
			{box !== null && (
				<div
					aria-hidden="true"
					data-tour-outline=""
					className="pointer-events-none fixed z-50 rounded-sm outline-2 outline-ring"
					style={{
						left: box.left,
						top: box.top,
						width: box.width,
						height: box.height,
					}}
				/>
			)}
			<section
				ref={card}
				role="dialog"
				aria-modal="false"
				aria-labelledby={titleId}
				aria-describedby={bodyId}
				data-tour-card=""
				data-tour-step={current.id}
				className="fixed z-50 w-80 max-w-[calc(100vw-2rem)] rounded-md border border-border bg-popover p-4 text-sm text-popover-foreground shadow-md"
				style={cardStyle(box, current.placement)}
				onKeyDown={onKeyDown}
			>
				<div aria-live="polite">
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.tour.step", {
							current: index + 1,
							total: TOUR_STEPS.length,
						})}
					</p>
					<h2 id={titleId} className="mt-1 text-sm font-semibold">
						{t(`ui.editor.tour.steps.${current.id}.title`)}
					</h2>
					<p id={bodyId} className="mt-1">
						{t(bodyKey, params)}
					</p>
				</div>
				<div className="mt-3 flex items-center justify-between gap-2">
					<Button type="button" size="sm" variant="ghost" onClick={endTour}>
						{t("ui.editor.tour.skip")}
					</Button>
					<div className="flex gap-2">
						{index > 0 && (
							<Button
								type="button"
								size="sm"
								variant="outline"
								onClick={() => goToTourStep(index - 1)}
							>
								{t("ui.editor.tour.back")}
							</Button>
						)}
						<Button
							type="button"
							size="sm"
							data-tour-next=""
							onClick={() => (last ? endTour() : goToTourStep(index + 1))}
						>
							{last ? t("ui.editor.tour.done") : t("ui.editor.tour.next")}
						</Button>
					</div>
				</div>
			</section>
		</>
	);
}
