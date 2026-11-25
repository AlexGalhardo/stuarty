import { useEffect, useState } from "react";
import { fetchProbe } from "../api/audioPreview";
import { audioPreviewUrl } from "../api/urls";
import { fpsOf } from "../commands/units";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { selectFps } from "../state/selectors";
import { columnPeaks, loadPeaks } from "../timeline/items/waveform";
import { processedPeaks } from "./voicePeaks";

export const WAVE_COLUMNS = 96;
const POLL_MS = 600;
const POLLS = 40;

export interface VoiceWaves {
	// One min and one max for each column; null while it is read or when there is nothing to draw.
	before: Float32Array | null;
	after: Float32Array | null;
	// The key of the processed file the server renders, when the item has a chain; null when it has none.
	processedKey: string | null;
}

// The waveforms of the studio voice screen: the sound of the item before the chain (the peaks of its asset) and
// after it (the processed sound the server renders, SDD-004 section 7.1). Both are read again when the project
// moves to a new revision, so applying a preset shows its effect.
export function useVoiceWaves(itemId: string): VoiceWaves {
	const runtime = useRuntime();
	const revision = useDocumentState((s) => s.revision);
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const fps = fpsOf(useDocumentState(selectFps));
	const [waves, setWaves] = useState<VoiceWaves>({
		before: null,
		after: null,
		processedKey: null,
	});
	const asset = item?.asset;
	const start = item?.start ?? 0;
	const duration = item?.duration ?? 0;
	const sourceIn = item?.source_in ?? 0;
	const speed = item?.speed ?? { num: 1, den: 1 };
	// biome-ignore lint/correctness/useExhaustiveDependencies: the numbers of the item stand for the item; revision is the trigger
	useEffect(() => {
		if (asset === undefined || duration < 1) return;
		let current = true;
		const run = async (): Promise<void> => {
			const peaks = await loadPeaks(runtime.projectId, asset);
			const before =
				peaks === null
					? null
					: columnPeaks(peaks, {
							sourceIn,
							speed: { num: speed.num, den: speed.den },
							fps,
							pxPerFrame: WAVE_COLUMNS / duration,
							firstColumn: 0,
							columns: WAVE_COLUMNS,
						});
			if (!current) return;
			setWaves({ before, after: before, processedKey: null });
			const plan = await runtime.client.getPlan(
				runtime.projectId,
				start,
				start + duration,
				runtime.documents.getState().revision ?? revision ?? 0,
			);
			const processed = plan.audio.find((a) => a.item === itemId)?.processed;
			if (!current || processed === undefined) return;
			const url = audioPreviewUrl(runtime.projectId, processed.key);
			for (let i = 0; i < POLLS && current; i++) {
				const probe = await fetchProbe(url);
				if (probe.kind === "failed") return;
				if (probe.kind === "ready") {
					const after = await processedPeaks(url, WAVE_COLUMNS);
					if (current && after !== null) {
						setWaves({ before, after, processedKey: processed.key });
					}
					return;
				}
				await new Promise((resolve) => setTimeout(resolve, POLL_MS));
			}
		};
		void run();
		return () => {
			current = false;
		};
	}, [
		runtime,
		itemId,
		asset,
		start,
		duration,
		sourceIn,
		speed.num,
		speed.den,
		revision,
	]);
	return waves;
}
