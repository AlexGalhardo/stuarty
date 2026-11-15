// The weight the core uses when a style names none (core/command/style.go).
const DEFAULT_FONT_WEIGHT = 700;

// The partial style for picking a family: when the family lacks the weight the
// item has now (or the core default), the nearest weight the family lists goes
// in the same operation, so the core never refuses the pick with
// E_FONT_UNKNOWN. A tie goes to the heavier weight. This chooses an argument;
// it computes no edit result.
export function fontFamilyChanges(
	family: string,
	weights: readonly number[],
	current: number | undefined,
): { font_family: string; font_weight?: number } {
	const wanted = current ?? DEFAULT_FONT_WEIGHT;
	if (weights.length === 0 || weights.includes(wanted)) {
		return { font_family: family };
	}
	let best = weights[0] as number;
	for (const weight of weights) {
		const gap = Math.abs(weight - wanted);
		const bestGap = Math.abs(best - wanted);
		if (gap < bestGap || (gap === bestGap && weight > best)) best = weight;
	}
	return { font_family: family, font_weight: best };
}
