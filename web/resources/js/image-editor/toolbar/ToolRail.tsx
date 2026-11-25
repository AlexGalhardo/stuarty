import { Ellipsis } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { useT } from "@/lib/i18n";
import { findLayer, useImage } from "../state/context";
import { type Operation, PRIMARY_TOOLS, useOperations } from "./operations";
import { SHORTCUTS } from "./shortcuts";
import ToolButton from "./ToolButton";
import { Toolbar } from "./Toolbar";

// The operations behind "more": every one the rail does not show as a primary tool.
export function moreOperations(operations: Operation[]): Operation[] {
	const primary: readonly string[] = PRIMARY_TOOLS;
	return operations.filter(
		(o) =>
			!primary.includes(o.id) && o.group !== "history" && o.group !== "view",
	);
}

// The primary tool the selection is of, so the rail shows what is being edited: a text layer lights Text, a
// rectangle Rectangle, a fill Fill, an image Image. It only reads the selected layer; it decides nothing.
function useActiveTool(): string | null {
	const doc = useImage((s) => s.doc);
	const selection = useImage((s) => s.selection);
	if (doc === null || selection.length !== 1) return null;
	const layer = findLayer(doc.layers, selection[0] ?? "");
	switch (layer?.kind) {
		case "text":
			return "add-text";
		case "image":
			return "add-image";
		case "fill":
			return "add-fill";
		case "shape":
			return layer.shape === "ellipse" ? null : "add-rect";
		default:
			return null;
	}
}

// The tool rail of the final design: the eight primary tools as icons and a "more" control for the rest, so every
// tool the editor has is still one click (or the palette) away. The buttons and the menu run the operations the
// palette lists.
export function ToolRail(): ReactNode {
	const t = useT();
	const operations = useOperations();
	const primary = PRIMARY_TOOLS.flatMap((id) => {
		const found = operations.find((o) => o.id === id);
		return found === undefined ? [] : [found];
	});
	const more = moreOperations(operations);
	const active = useActiveTool();
	const hasSelection = useImage((s) => s.selection.length > 0);
	const hasDocument = useImage((s) => s.doc !== null);

	return (
		<Toolbar label={t("ui.image.toolbar.label")} orientation="vertical">
			{primary.map((o) => (
				<ToolButton
					key={o.id}
					icon={o.icon}
					label={o.label}
					shortcut={o.shortcut}
					disabled={o.disabled}
					pressed={active === o.id}
					onClick={o.run}
					testId={o.testId}
				/>
			))}
			<DropdownMenu>
				<Tooltip>
					<TooltipTrigger asChild>
						<DropdownMenuTrigger asChild>
							<Button
								type="button"
								variant="ghost"
								size="icon"
								aria-label={t("ui.image.toolbar.more")}
								className="mt-1"
								data-tool=""
								data-testid="image-more-tools"
								// The arrows walk the rail (Toolbar.tsx); a menu trigger would open on ArrowDown, so
								// here only Enter, Space and the pointer open it.
								onKeyDown={(event) => {
									if (event.key === "ArrowDown" || event.key === "ArrowUp") {
										(
											event as unknown as { preventBaseUIHandler?: () => void }
										).preventBaseUIHandler?.();
									}
								}}
							>
								<Ellipsis aria-hidden="true" />
							</Button>
						</DropdownMenuTrigger>
					</TooltipTrigger>
					<TooltipContent>{t("ui.image.toolbar.more")}</TooltipContent>
				</Tooltip>
				<DropdownMenuContent
					side="right"
					align="start"
					sideOffset={8}
					className="w-60"
				>
					{more.map((o) => {
						const Icon = o.icon;
						const keys =
							o.shortcut === undefined ? undefined : SHORTCUTS[o.shortcut];
						return (
							<DropdownMenuItem
								key={o.id}
								disabled={o.disabled}
								className="h-8 data-disabled:opacity-60"
								data-testid={o.testId}
								aria-keyshortcuts={keys}
								onSelect={o.run}
							>
								<Icon aria-hidden="true" />
								<span className="flex-1">{o.label}</span>
								{keys !== undefined && (
									<kbd className="font-sans text-xs text-muted-foreground">
										{keys}
									</kbd>
								)}
							</DropdownMenuItem>
						);
					})}
					{hasDocument && !hasSelection && (
						<p className="m-0 border-t border-border px-2 pt-2 pb-1 text-xs text-text-muted">
							{t("ui.image.toolbar.more_hint")}
						</p>
					)}
				</DropdownMenuContent>
			</DropdownMenu>
		</Toolbar>
	);
}
