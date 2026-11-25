// The pictures of the studio voice screen (final design 30): the waveform of the sound before the chain and after it.
// They are decoration: a min and a max per column, drawn from samples the service rendered. Nothing here changes a
// sample or decides what the processors do.

// One min and one max per column, each in [-1, 1], from the samples of a channel.
export function bucketPeaks(
	samples: Float32Array,
	columns: number,
): Float32Array {
	const out = new Float32Array(columns * 2);
	if (samples.length === 0 || columns < 1) return out;
	for (let c = 0; c < columns; c++) {
		const from = Math.floor((c * samples.length) / columns);
		const to = Math.max(
			from + 1,
			Math.floor(((c + 1) * samples.length) / columns),
		);
		let lo = 0;
		let hi = 0;
		for (let i = from; i < to && i < samples.length; i++) {
			const v = samples[i] as number;
			if (v < lo) lo = v;
			if (v > hi) hi = v;
		}
		out[c * 2] = lo;
		out[c * 2 + 1] = hi;
	}
	return out;
}

// The processed sound of an item, as the server renders it (SDD-004 section 7.1): read whole and decoded, then
// reduced to columns. null when the browser cannot decode it; the stage then draws no picture and says so.
export async function processedPeaks(
	url: string,
	columns: number,
): Promise<Float32Array | null> {
	try {
		const response = await fetch(url, { credentials: "same-origin" });
		if (!response.ok) return null;
		const context = new OfflineAudioContext(1, 1, 48000);
		const decoded = await context.decodeAudioData(await response.arrayBuffer());
		return bucketPeaks(decoded.getChannelData(0), columns);
	} catch {
		return null;
	}
}
