// The clear of one uncovered preview pixel, RGBA 0..255. The plan says
// `transparent` when the project background is the word "transparent" (the
// colour is then #000000, SDD-001). Either signal leaves the pixel clear.
// PixiExecutor is the only painter: it clears with this and draws layers on top.
export function uncoveredPixel(source: {
	background?: string;
	transparent?: boolean;
}): readonly [number, number, number, number] {
	if (source.transparent === true || source.background === "transparent") {
		return [0, 0, 0, 0];
	}
	const match = /^#([0-9a-fA-F]{6})([0-9a-fA-F]{2})?$/.exec(
		(source.background ?? "#000000").trim(),
	);
	if (match === null) return [0, 0, 0, 255];
	const rgb = Number.parseInt(match[1] as string, 16);
	const alpha = match[2] === undefined ? 255 : Number.parseInt(match[2], 16);
	return [(rgb >> 16) & 255, (rgb >> 8) & 255, rgb & 255, alpha];
}
