import registry from "@/generated/commands.json";

// The named blur strengths of the core (SDD-002 C.90) as the registry carries them: the `x-vv-blur-strengths`
// extension of the `strength` argument of clip.blur_region. The control is drawn from this table and holds no
// radius of its own: applying one sends its id and the core resolves the radius for the size of the project.

export interface BlurStrengthEntry {
	readonly id: string;
	readonly name: string;
	// The share of the shorter side of the project the radius is.
	readonly share: number;
}

interface Schema {
	readonly properties?: Readonly<Record<string, Schema>>;
	readonly "x-vv-blur-strengths"?: readonly BlurStrengthEntry[];
}

const blurRegion = registry.find((c) => c.name === "clip.blur_region")
	?.args_schema as unknown as Schema | undefined;

export const BLUR_STRENGTHS: readonly BlurStrengthEntry[] =
	blurRegion?.properties?.strength?.["x-vv-blur-strengths"] ?? [];
