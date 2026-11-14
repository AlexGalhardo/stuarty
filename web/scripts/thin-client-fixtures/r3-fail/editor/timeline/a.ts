import type { Item } from "@/generated/timeline";
export function nudge(item: Item): void {
	item.start = 5;
}
