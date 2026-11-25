import { cn } from "@/lib/cn";
import type { Drop } from "./tree";

// The look of one row of the layer panel, in the language of the rows of the dashboard lists (ListRow): a quiet
// hover, the selected tint with an accent bar at the start edge (so selection is not colour alone; the
// `--ed-selection` token is white in the light theme and would not show), a dimmed row for a hidden
// layer and the drop marks of a drag. The row is a `group`, so its toggles can show on hover and on focus.
export function rowClass({
	selected,
	visible,
	mark,
}: {
	selected: boolean;
	visible: boolean;
	mark: Drop | null;
}): string {
	return cn(
		"group relative flex h-8 scroll-mt-12 cursor-default items-center gap-1 rounded-md pr-1 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring",
		selected
			? "bg-surface-selected text-foreground shadow-[inset_2px_0_0_var(--color-accent)]"
			: "hover:bg-surface-hover",
		mark === "inside" && "ring-2 ring-ed-handle",
		mark === "above" && "border-t-2 border-ed-handle",
		mark === "below" && "border-b-2 border-ed-handle",
		!visible && "opacity-60",
	);
}

// The toggles at the end of the row. An unlocked layer shows its open padlock only while the row is hovered,
// focused or selected, so a column of open padlocks is not drawn as noise; a locked one always shows.
export function lockClass(locked: boolean): string {
	return cn(
		"size-4",
		!locked &&
			"opacity-0 group-hover:opacity-70 group-focus-visible:opacity-70 group-aria-selected:opacity-70",
	);
}
