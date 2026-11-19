import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";

// One icon button of the toolbar (design system 4.3.1): an icon, an accessible name and a tooltip with the same
// words, and the key it has when it has one. The toolbar makes one of them the tab stop (Toolbar.tsx).
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
	shortcut?: string;
	testId?: string;
}): ReactNode {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					aria-label={label}
					aria-keyshortcuts={shortcut}
					aria-pressed={pressed}
					disabled={disabled}
					data-tool=""
					data-testid={testId}
					onClick={onClick}
				>
					<Icon aria-hidden="true" />
				</Button>
			</TooltipTrigger>
			<TooltipContent>
				{label}
				{shortcut !== undefined && (
					<span className="ml-2 text-muted-foreground">{shortcut}</span>
				)}
			</TooltipContent>
		</Tooltip>
	);
}
