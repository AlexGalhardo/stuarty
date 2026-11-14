import type { Item } from "@/generated/timeline";
export function apply(item: Item): void {
	item.start = 5;
}
