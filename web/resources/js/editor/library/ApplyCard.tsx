import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";

// One card of the grids of the material tabs (Transitions, Filters, Effects):
// the name, and one command on click. With nothing to act on it is
// `aria-disabled`, with the reason in the paragraph `why` names.
export default function ApplyCard({
	name,
	enabled,
	why,
	pressed,
	onApply,
	children,
}: {
	name: string;
	enabled: boolean;
	why: string;
	pressed?: boolean;
	onApply(): void;
	children?: ReactNode;
}): ReactNode {
	return (
		<Button
			type="button"
			variant="outline"
			aria-disabled={!enabled}
			aria-describedby={enabled ? undefined : why}
			aria-pressed={pressed}
			className={cn(
				"h-16 flex-col items-start justify-end whitespace-normal px-2 py-1.5 text-left",
				!enabled && "opacity-60",
			)}
			onClick={() => {
				if (enabled) onApply();
			}}
		>
			{children}
			<span>{name}</span>
		</Button>
	);
}
