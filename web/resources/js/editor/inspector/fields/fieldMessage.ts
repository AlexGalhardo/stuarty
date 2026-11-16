// A message the core wrote for an operation names its argument ("args.crop:
// ..."), which is an internal path to a person editing in the inspector.
export function withoutArgPath(message: string): string {
	return message.replace(/^args(?:\.[\w[\]]+)+:\s*/, "");
}

// The limits a number field refused a value for, carried in `details`.
export function rangeOf(
	details: { readonly [key: string]: unknown } | undefined,
): { min: number; max: number } | null {
	const min = details?.min;
	const max = details?.max;
	return typeof min === "number" && typeof max === "number"
		? { min, max }
		: null;
}
