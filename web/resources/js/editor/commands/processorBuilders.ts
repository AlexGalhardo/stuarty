import type { Operations } from "./builders";
import type { AudioPresetId, ProcessorKind } from "./processorTable";

// The operations of the audio processor chain of an item or a track (SDD-004 section 7.1, SDD-002 C.79 to C.83). The
// inspector requests; the core checks the ranges and decides (rules T5 and T6).

export type ProcessorTarget =
	| { readonly itemId: string }
	| { readonly trackId: string };

function targetArgs(
	target: ProcessorTarget,
): { item_id: string } | { track_id: string } {
	return "itemId" in target
		? { item_id: target.itemId }
		: { track_id: target.trackId };
}

export function processorAddOperations(
	target: ProcessorTarget,
	kind: ProcessorKind,
): Operations {
	return [{ op: "audio.processor_add", args: { ...targetArgs(target), kind } }];
}

export interface ProcessorChange {
	readonly params?: Readonly<Record<string, number>>;
	readonly enabled?: boolean;
}

export function processorSetOperations(
	target: ProcessorTarget,
	index: number,
	change: ProcessorChange,
): Operations {
	return [
		{
			op: "audio.processor_set",
			args: {
				...targetArgs(target),
				index,
				...(change.params === undefined
					? {}
					: { params: { ...change.params } }),
				...(change.enabled === undefined ? {} : { enabled: change.enabled }),
			},
		},
	];
}

export function processorMoveOperations(
	target: ProcessorTarget,
	index: number,
	to: number,
): Operations {
	return [
		{ op: "audio.processor_move", args: { ...targetArgs(target), index, to } },
	];
}

export function processorRemoveOperations(
	target: ProcessorTarget,
	index: number,
): Operations {
	return [
		{ op: "audio.processor_remove", args: { ...targetArgs(target), index } },
	];
}

export function applyPresetOperations(
	target: ProcessorTarget,
	preset: AudioPresetId,
): Operations {
	return [
		{ op: "audio.apply_preset", args: { ...targetArgs(target), preset } },
	];
}
