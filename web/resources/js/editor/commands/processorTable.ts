import type {
	AudioApplyPresetArgs,
	AudioProcessorAddArgs,
} from "@/generated/commands";
import registry from "@/generated/commands.json";

// The audio processors of the core as the registry carries them (SDD-004 section 7.1): the kinds, the unit, range and
// default of each parameter, and the presets. The editor draws its controls from this table and states no range of
// its own; the table is the `x-vv-processors` extension of the `params` argument of audio.processor_add.

export type ProcessorKind = AudioProcessorAddArgs["kind"];
export type AudioPresetId = AudioApplyPresetArgs["preset"];

export interface ProcessorParam {
	readonly name: string;
	readonly unit: string;
	readonly min: number;
	readonly max: number;
	readonly default: number;
	readonly off_zero?: boolean;
	readonly min_on?: number;
}

export interface ProcessorSpec {
	readonly kind: ProcessorKind;
	readonly params: readonly ProcessorParam[];
}

interface Schema {
	readonly properties?: Readonly<Record<string, Schema>>;
	readonly enum?: readonly string[];
	readonly "x-vv-processors"?: readonly ProcessorSpec[];
	readonly "x-vv-max-processors"?: number;
}

function schemaOf(command: string): Schema | undefined {
	return registry.find((c) => c.name === command)?.args_schema as unknown as
		| Schema
		| undefined;
}

const params = schemaOf("audio.processor_add")?.properties?.params;

export const AUDIO_PROCESSORS: readonly ProcessorSpec[] =
	params?.["x-vv-processors"] ?? [];

// The longest chain of an item or a track.
export const MAX_PROCESSORS: number = params?.["x-vv-max-processors"] ?? 0;

export const AUDIO_PRESETS: readonly AudioPresetId[] = (schemaOf(
	"audio.apply_preset",
)?.properties?.preset?.enum ?? []) as readonly AudioPresetId[];
