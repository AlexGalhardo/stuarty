import type { ReactNode } from "react";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { cn } from "@/lib/cn";
import type { Option } from "./OptionSelect";

// A segmented choice of one among a few, for a value that changes what a screen shows or what a form asks
// (the kind of a text, the range of a calendar). It is a group of toggle buttons named by `label`; the chosen
// one carries aria-pressed. Choosing the chosen one again keeps it: a segment never ends with nothing chosen.
export function Segment({
	value,
	onChange,
	options,
	label,
	className,
}: {
	value: string;
	onChange(value: string): void;
	options: readonly Option[];
	label: string;
	className?: string;
}): ReactNode {
	return (
		<ToggleGroup
			type="single"
			variant="outline"
			spacing={0}
			value={value}
			onValueChange={(next) => {
				if (typeof next === "string" && next !== "") onChange(next);
			}}
			aria-label={label}
			data-slot="segment"
			className={cn("flex-wrap", className)}
		>
			{options.map((o) => (
				<ToggleGroupItem key={o.value} value={o.value} className="px-3">
					{o.label}
				</ToggleGroupItem>
			))}
		</ToggleGroup>
	);
}
