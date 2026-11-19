import type { Translate } from "@/lib/i18n";
import type {
	BatchItem,
	ExportBatchItem,
	ExportOptions,
	ExportPreset,
} from "../api/types";

// The platform presets of the core (SDD-004 section 8.2) as the dialog shows
// them. Every limit is the core's: this file only groups and words them, and
// never decides whether a project fits (that is ExportCheck).

export interface NetworkGroup {
	network: string;
	presets: ExportPreset[];
}

// The presets by network, in the order the core lists them.
export function groupByNetwork(
	presets: readonly ExportPreset[],
): NetworkGroup[] {
	const groups: NetworkGroup[] = [];
	for (const preset of presets) {
		const group = groups.find((g) => g.network === preset.network);
		if (group === undefined)
			groups.push({ network: preset.network, presets: [preset] });
		else group.presets.push(preset);
	}
	return groups;
}

function seconds(ms: number): string {
	return String(Math.round((ms / 1000) * 1000) / 1000);
}

function size(bytes: number): string {
	return bytes >= 1_000_000_000
		? `${Math.round(bytes / 1e7) / 100} GB`
		: `${Math.round(bytes / 1e5) / 10} MB`;
}

// The limits of a preset as short phrases, in the order of the check.
export function limitLines(t: Translate, preset: ExportPreset): string[] {
	const l = preset.limits;
	const lines: string[] = [];
	if (l.min_duration_ms !== undefined && l.max_duration_ms !== undefined) {
		lines.push(
			t("ui.editor.export.presets.limit_duration_range", {
				min: seconds(l.min_duration_ms),
				max: seconds(l.max_duration_ms),
			}),
		);
	} else if (l.max_duration_ms !== undefined) {
		lines.push(
			t("ui.editor.export.presets.limit_duration_max", {
				seconds: seconds(l.max_duration_ms),
			}),
		);
	} else if (l.min_duration_ms !== undefined) {
		lines.push(
			t("ui.editor.export.presets.limit_duration_min", {
				seconds: seconds(l.min_duration_ms),
			}),
		);
	}
	if (l.min_fps !== undefined && l.max_fps !== undefined) {
		lines.push(
			t("ui.editor.export.presets.limit_fps", {
				min: l.min_fps,
				max: l.max_fps,
			}),
		);
	} else if (l.max_fps !== undefined) {
		lines.push(t("ui.editor.export.presets.limit_fps_max", { max: l.max_fps }));
	}
	if (l.aspect_ratio !== undefined && l.aspect_ratio !== "") {
		lines.push(
			t("ui.editor.export.presets.limit_aspect", { ratio: l.aspect_ratio }),
		);
	}
	if (l.max_bytes !== undefined) {
		lines.push(
			t("ui.editor.export.presets.limit_size", { size: size(l.max_bytes) }),
		);
	}
	return lines;
}

// The exports of a batch: one per chosen preset, with the codec the person
// chose (empty is the preset's own preference) and the range of the dialog.
export function batchItems(
	selected: readonly string[],
	codec: string,
	options: ExportOptions,
): ExportBatchItem[] {
	return selected.map((id) => ({
		preset: id,
		codec: codec === "" ? undefined : codec,
		start: options.start,
		end: options.end,
	}));
}

// The one export of a batch that stands for a plain export which the server had
// no free slot for: the core queues it behind the others (VV-220).
export function queuedItem(options: ExportOptions): ExportBatchItem {
	return {
		format: options.format,
		codec: options.codec,
		start: options.start,
		end: options.end,
	};
}

// The name of a batch item in a list: the preset label, else the format.
export function itemName(
	item: Pick<BatchItem, "preset" | "format">,
	presets: readonly ExportPreset[],
	t: Translate,
): string {
	if (item.preset !== undefined) {
		return presets.find((p) => p.id === item.preset)?.label ?? item.preset;
	}
	return item.format === "mp4"
		? t("ui.editor.export.batch.plain")
		: item.format;
}
