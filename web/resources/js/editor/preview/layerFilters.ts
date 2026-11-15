// The plan carries a colour matrix and a filter matrix as 3x4 rows of
// `r g b offset` over values in 0..1 (SDD-004 section 3.5, 3.6). PixiJS wants
// 5x4 rows of `r g b a offset` with the offset in 0..1 too, so the matrix gets
// an alpha column of zeros and a row that leaves alpha alone.
export function pixiColorMatrix(rows: readonly number[]): number[] {
	if (rows.length !== 12) {
		throw new Error(`a plan matrix has 12 numbers, got ${rows.length}`);
	}
	const out: number[] = [];
	for (let row = 0; row < 3; row++) {
		out.push(rows[row * 4], rows[row * 4 + 1], rows[row * 4 + 2], 0);
		out.push(rows[row * 4 + 3]);
	}
	out.push(0, 0, 0, 1, 0);
	return out;
}
