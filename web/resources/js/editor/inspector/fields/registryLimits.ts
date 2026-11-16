import registry from "@/generated/commands.json";

export interface Limits {
	min?: number;
	max?: number;
}

interface Schema {
	minimum?: number;
	maximum?: number;
	properties?: Record<string, Schema>;
}

// The numeric limits the core's registry gives an argument of a command (the
// `minimum` and `maximum` of its schema, generated from Go), so a field refuses
// what the core would refuse without the editor repeating the numbers. `path`
// names the argument, one key per level (`["crop", "left"]`). A command or
// argument the registry does not know has no limits.
export function registryLimits(command: string, ...path: string[]): Limits {
	let schema: Schema | undefined = registry.find((c) => c.name === command)
		?.args_schema as unknown as Schema | undefined;
	for (const key of path) schema = schema?.properties?.[key];
	const out: Limits = {};
	if (typeof schema?.minimum === "number") out.min = schema.minimum;
	if (typeof schema?.maximum === "number") out.max = schema.maximum;
	return out;
}

// Both limits, for a control that needs them (a slider). The registry always
// carries them for the arguments a slider edits; a missing one is a mismatch
// between the editor and the core and fails loudly, in registryLimits.test.ts.
export function registryRange(
	command: string,
	...path: string[]
): Required<Limits> {
	const { min, max } = registryLimits(command, ...path);
	if (min === undefined || max === undefined) {
		throw new Error(
			`${command} ${path.join(".")} has no range in the registry`,
		);
	}
	return { min, max };
}
