// Scrolls the cell of an item into view. A virtualized row (SDD-012 section
// 7.9) mounts an item outside its window only once it is selected, focused or
// outlined, which the render after the caller's store write does; so a cell
// that is not there yet is looked up once more on the next frame.
export function revealItem(
	itemId: string,
	options: ScrollIntoViewOptions = { block: "nearest", inline: "nearest" },
): void {
	const find = (): HTMLElement | null =>
		document.querySelector<HTMLElement>(
			`[data-item-id="${CSS.escape(itemId)}"]`,
		);
	const cell = find();
	if (cell !== null) {
		cell.scrollIntoView(options);
		return;
	}
	requestAnimationFrame(() => find()?.scrollIntoView(options));
}
