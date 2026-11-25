import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/cn";
import { SHORTCUTS, type ShortcutRow } from "./shortcuts";

// One icon button of the toolbar (design system 4.3.1): an icon, an accessible name and a tooltip with the same
// words, and the key it has when it has one, read from the table of shortcuts. The toolbar makes one of them
// the tab stop (Toolbar.tsx).
export default function ToolButton({
	icon: Icon,
	label,
	onClick,
	disabled,
	pressed,
	shortcut,
	testId,
}: {
	icon: LucideIcon;
	label: string;
	onClick: () => void;
	disabled?: boolean;
	pressed?: boolean;
	shortcut?: ShortcutRow;
	testId?: string;
}): ReactNode {
	const keys = shortcut === undefined ? undefined : SHORTCUTS[shortcut];
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-label={label}
					aria-keyshortcuts={keys}
					aria-pressed={pressed}
					disabled={disabled}
					className={cn(
						pressed && "bg-accent-soft text-accent-text hover:bg-accent-soft",
					)}
					data-tool=""
					data-testid={testId}
					onClick={onClick}
				>
					<Icon aria-hidden="true" />
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
