import type { JsonValue } from "../../api/types";

// A JSON Schema 2020-12 sub-schema of an `args_schema` (SDD-012 section 9.2).
export type Schema = { readonly [key: string]: unknown };

export type IdKind = "asset" | "track" | "item";

// One renderer per construct of the table of section 9.2.
export type Construct =
	| { kind: "id"; idKind: IdKind }
	| { kind: "time" }
	| { kind: "enum"; values: readonly JsonValue[] }
	| { kind: "text" }
	| { kind: "number"; integer: boolean; min?: number; max?: number }
	| { kind: "boolean" }
	| { kind: "list"; items: Schema; itemKey: string }
	| { kind: "object"; fields: readonly Field[] }
	| {
			kind: "variants";
			keyword: "oneOf" | "anyOf";
			variants: readonly Schema[];
	  }
	// Anything the generator cannot render: a JSON textarea for that sub-value.
	| { kind: "json" };

export interface Field {
	name: string;
	schema: Schema;
	required: boolean;
}

const isSchema = (value: unknown): value is Schema =>
	typeof value === "object" && value !== null && !Array.isArray(value);

const arrayOf = (value: unknown): readonly unknown[] =>
	Array.isArray(value) ? value : [];

// The registry does not mark time arguments yet (`x-vv-kind: "time"` is what
// SDD-012 9.2 expects), so a string is also a time when its description says
// so, in the words the registry uses for A.4 arguments, or when it has the
// name of a time argument of the catalogue.
const TIME_DESCRIPTION =
	/\b(time with a unit|length|absolute (?:position|start|frame)|offset into)\b/i;
const TIME_NAMES = new Set([
	"start",
	"duration",
	"at",
	"to",
	"source_in",
	"fade_in",
	"fade_out",
	"max_gap",
	"min_silence",
	"padding",
]);

export function isTimeSchema(schema: Schema, name: string): boolean {
	if (schema["x-vv-kind"] === "time") return true;
	if (schema.format === "time" || schema.format === "duration") return true;
	const description =
		typeof schema.description === "string" ? schema.description : "";
	return TIME_DESCRIPTION.test(description) || TIME_NAMES.has(name);
}

const ID_NAME = /^(?:.*_)?(asset|track|item)(?:_ids?)?$/;
const PREFIX_KIND: Readonly<Record<string, IdKind>> = {
	ast_: "asset",
	trk_: "track",
	itm_: "item",
};

// The kind of object an id argument names: from a pattern with an `ast_`,
// `trk_` or `itm_` prefix, else from the argument name (`asset_id`,
// `from_item_id`, `item_ids`, `linked_item`, `above`, `below`). `id` and
// `new_id` name something to create, so they are plain text.
export function idKindOf(schema: Schema, name: string): IdKind | null {
	if (typeof schema.pattern === "string") {
		for (const [prefix, kind] of Object.entries(PREFIX_KIND)) {
			if (schema.pattern.includes(prefix)) return kind;
		}
	}
	if (name === "above" || name === "below") return "track";
	if (name === "linked_item") return "item";
	const match = /_ids?$/.test(name) ? ID_NAME.exec(name) : null;
	return match === null ? null : (match[1] as IdKind);
}

export function fieldsOf(schema: Schema): Field[] {
	const properties = isSchema(schema.properties) ? schema.properties : {};
	const required = new Set(arrayOf(schema.required));
	return Object.entries(properties)
		.filter((entry): entry is [string, Schema] => isSchema(entry[1]))
		.map(([name, sub]) => ({
			name,
			schema: sub,
			required: required.has(name),
		}));
}

const SCALARS = new Set(["string", "number", "integer", "boolean"]);

export function classify(schema: Schema, name: string): Construct {
	// The registry writes a value the core also accepts as null as
	// type: [T, "null"] (SDD-012 §9.2); the field is the control of T.
	if (Array.isArray(schema.type)) {
		const types = schema.type.filter((t) => t !== "null");
		if (types.length === 1)
			return classify({ ...schema, type: types[0] }, name);
		return { kind: "json" };
	}
	for (const keyword of ["oneOf", "anyOf"] as const) {
		const variants = arrayOf(schema[keyword]).filter(isSchema);
		if (variants.length > 0) return { kind: "variants", keyword, variants };
	}
	if (Array.isArray(schema.enum) && schema.enum.length > 0) {
		return { kind: "enum", values: schema.enum as JsonValue[] };
	}
	switch (schema.type) {
		case "string": {
			if (isTimeSchema(schema, name)) return { kind: "time" };
			const idKind = idKindOf(schema, name);
			return idKind === null ? { kind: "text" } : { kind: "id", idKind };
		}
		case "number":
		case "integer": {
			const construct: Construct = {
				kind: "number",
				integer: schema.type === "integer",
			};
			if (typeof schema.minimum === "number") construct.min = schema.minimum;
			if (typeof schema.maximum === "number") construct.max = schema.maximum;
			return construct;
		}
		case "boolean":
			return { kind: "boolean" };
		case "array": {
			if (!isSchema(schema.items)) return { kind: "json" };
			const items = schema.items;
			const scalar = typeof items.type === "string" && SCALARS.has(items.type);
			const object = items.type === "object" && fieldsOf(items).length > 0;
			return scalar || object || Array.isArray(items.enum)
				? { kind: "list", items, itemKey: name }
				: { kind: "json" };
		}
		case "object": {
			const fields = fieldsOf(schema);
			return fields.length > 0 ? { kind: "object", fields } : { kind: "json" };
		}
		default:
			return { kind: "json" };
	}
}
