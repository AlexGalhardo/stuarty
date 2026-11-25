import type { ReactNode } from "react";

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { Option } from "./OptionSelect";

// The tabs of a screen that holds one list in several views (All, Pending, Kept): a tab list with the line
// variant of the design system. It draws no panel: the page shows the view the `value` names, under the bar.
export function TabBar({
	value,
	onChange,
	tabs,
	label,
	className,
}: {
	value: string;
	onChange(value: string): void;
	tabs: readonly (Option & { count?: number })[];
	label: string;
	className?: string;
}): ReactNode {
	return (
		<Tabs value={value} onValueChange={onChange} className={className}>
			<TabsList variant="line" aria-label={label}>
				{tabs.map((tab) => (
					<TabsTrigger key={tab.value} value={tab.value}>
						{tab.label}
						{tab.count === undefined ? null : (
							<span className="tabular-nums text-text-subtle">{tab.count}</span>
						)}
					</TabsTrigger>
				))}
			</TabsList>
		</Tabs>
	);
}
