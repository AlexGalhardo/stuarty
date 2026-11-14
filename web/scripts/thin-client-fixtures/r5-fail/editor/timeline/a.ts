export function end(item: { start: number; duration: number }): number {
	return item.start + item.duration;
}
