import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { useT } from "@/lib/i18n";
import { effectiveZoom } from "../canvas/viewport";
import { useImage } from "../state/context";
import { type Operation, useOperations } from "./operations";
import { SHORTCUTS } from "./shortcuts";
import ToolButton from "./ToolButton";
import { Toolbar } from "./Toolbar";

// A button of the bar that says a word or a number, with the tooltip of its operation.
function TextTool({
	operation,
	label,
	testId,
	children,
}: {
	operation: Operation;
	label: string;
	testId: string;
	children: ReactNode;
}): ReactNode {
	const keys =
		operation.shortcut === undefined
			? undefined
			: SHORTCUTS[operation.shortcut];
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					aria-label={label}
					aria-keyshortcuts={keys}
					disabled={operation.disabled}
					data-tool=""
					data-testid={testId}
					onClick={operation.run}
				>
					{children}
				</Button>
			</TooltipTrigger>
			<TooltipContent>
				{label}
				{keys !== undefined && (
					<kbd className="ml-2 rounded-sm border border-border px-1 font-sans text-xs text-muted-foreground">
						{keys}
					</kbd>
				)}
			</TooltipContent>
		</Tooltip>
	);
}

// The zoom bar of the final design, centred under the image: zoom out, the percentage (a click on it goes to
// 100%), zoom in and Fit. The percentage is the zoom the viewport reports (canvas/viewport.ts); the steps and
// the limits are the ones the keys and the palette use, because every control runs the same operation.
export function ZoomBar(): ReactNode {
	const t = useT();
	const operations = useOperations();
	const doc = useImage((s) => s.doc);
	const view = useImage((s) => s.view);
	const stage = useImage((s) => s.stage);
	const find = (id: string): Operation => {
		const found = operations.find((o) => o.id === id);
		if (found === undefined) throw new Error(`unknown operation ${id}`);
		return found;
	};
	const zoomOut = find("zoom-out");
	const zoomIn = find("zoom-in");
	const fit = find("zoom-fit");
	const actual = find("zoom-actual");
	const percent =
		doc === null
			? null
			: Math.round(
					effectiveZoom(view, stage, { w: doc.width, h: doc.height }) * 100,
				);

	return (
		<Toolbar
			label={t("ui.image.toolbar.view")}
			testId="image-view-toolbar"
			className="justify-center gap-1 border-t-0 bg-ed-bg py-2"
		>
			<ToolButton
				icon={zoomOut.icon}
				label={zoomOut.label}
				shortcut="zoomOut"
				disabled={zoomOut.disabled}
				onClick={zoomOut.run}
				testId={zoomOut.testId}
			/>
			<TextTool
				operation={actual}
				label={
					percent === null
						? actual.label
						: t("ui.image.view.percent_button", { percent })
				}
				testId={actual.testId}
			>
				<span
					data-testid="image-zoom-percent"
					className="min-w-10 text-center text-xs tabular-nums"
				>
					{percent === null ? "" : `${percent}%`}
				</span>
			</TextTool>
			<ToolButton
				icon={zoomIn.icon}
				label={zoomIn.label}
				shortcut="zoomIn"
				disabled={zoomIn.disabled}
				onClick={zoomIn.run}
				testId={zoomIn.testId}
			/>
			<TextTool operation={fit} label={fit.label} testId={fit.testId}>
				{t("ui.image.view.fit_short")}
			</TextTool>
		</Toolbar>
	);
}
