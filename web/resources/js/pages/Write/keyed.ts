// A key for each item of a list that has no id of its own: the text of the item and how many items before it had the
// same text, so two equal paragraphs keep distinct keys and a key never depends on the position alone.
export function keyed<T>(
	items: readonly T[],
	text: (item: T) => string,
): { key: string; item: T }[] {
	const seen = new Map<string, number>();
	return items.map((item) => {
		const base = text(item);
		const before = seen.get(base) ?? 0;
		seen.set(base, before + 1);
		return { key: `${base}#${before}`, item };
	});
}
