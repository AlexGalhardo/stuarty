//! The luma SSIM of SDD-013 section 3.8.

/// The BT.709 luma of an RGBA frame, one byte per pixel, rounded as the Go test does.
pub fn luma(pix: &[u8]) -> Vec<u8> {
	pix.as_chunks::<4>()
		.0
		.iter()
		.map(|p| {
			let (r, g, b) = (u32::from(p[0]), u32::from(p[1]), u32::from(p[2]));
			((2126 * r + 7152 * g + 722 * b + 5000) / 10000) as u8
		})
		.collect()
}

/// The mean structural similarity of two luma planes over 8x8 windows every 4 pixels (the routine
/// of `engine/export/util_test.go`, SDD-004 section 11). Planes smaller than a window are equal
/// when their bytes are.
pub fn ssim(a: &[u8], b: &[u8], w: usize, h: usize) -> f64 {
	const C1: f64 = (0.01 * 255.0) * (0.01 * 255.0);
	const C2: f64 = (0.03 * 255.0) * (0.03 * 255.0);
	let (mut sum, mut count) = (0.0, 0_u32);
	let mut y = 0;
	while y + 8 <= h {
		let mut x = 0;
		while x + 8 <= w {
			let (mut sa, mut sb, mut saa, mut sbb, mut sab) = (0.0, 0.0, 0.0, 0.0, 0.0);
			for j in 0..8 {
				for i in 0..8 {
					let k = (y + j) * w + x + i;
					let (va, vb) = (f64::from(a[k]), f64::from(b[k]));
					sa += va;
					sb += vb;
					saa += va * va;
					sbb += vb * vb;
					sab += va * vb;
				}
			}
			let (ma, mb) = (sa / 64.0, sb / 64.0);
			let (va, vb, cov) = (
				saa / 64.0 - ma * ma,
				sbb / 64.0 - mb * mb,
				sab / 64.0 - ma * mb,
			);
			sum += (2.0 * ma * mb + C1) * (2.0 * cov + C2)
				/ ((ma * ma + mb * mb + C1) * (va + vb + C2));
			count += 1;
			x += 4;
		}
		y += 4;
	}
	if count == 0 {
		return if a == b { 1.0 } else { 0.0 };
	}
	sum / f64::from(count)
}
