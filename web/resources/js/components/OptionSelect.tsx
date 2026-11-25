import type { ReactNode } from "react";

import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";

export interface Option {
	value: string;
	label: string;
}

// The styled select of the design system for a short list of options: the page passes the options and the
// accessible name and gets the same control everywhere, instead of a native <select>. It only reports the
// choice (design-system section 4.3). `size="sm"` is the 28 px compact form of a filter row.
export function OptionSelect({
	value,
	onChange,
	options,
	label,
	size = "default",
	className,
	disabled,
	name,
}: {
	value: string;
	onChange(value: string): void;
	options: readonly Option[];
	// The accessible name of the control; a visible label beside it is the caller's.
	label: string;
	size?: "sm" | "default";
	className?: string;
	disabled?: boolean;
	name?: string;
}): ReactNode {
	return (
		<Select
			value={value}
			onValueChange={onChange}
			disabled={disabled}
			name={name}
		>
			<SelectTrigger size={size} aria-label={label} className={className}>
				<SelectValue>
					{(current: string) =>
						options.find((o) => o.value === current)?.label ?? current
					}
				</SelectValue>
			</SelectTrigger>
			<SelectContent position="popper" align="start">
				{options.map((o) => (
					<SelectItem key={o.value} value={o.value}>
						{o.label}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	);
}
