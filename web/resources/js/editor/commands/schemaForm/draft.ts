import type { JsonValue } from "../../api/types";
import {
	localizeDecimalInput,
	normalizeTimeInput,
	type TimeUnit,
} from "../units";
import { classify, type Schema } from "./classify";

// What the form keeps while the person types: the text of a time field and of a
// JSON textarea as typed, numbers and booleans as values, lists as arrays, and
// the chosen variant of a `oneOf` or `anyOf`. `finalize` turns it into the
// arguments of the command, or says which fields are not valid yet.
export type Draft =
	| string
	| number
	| boolean
	| undefined
	| Draft[]
	| { [key: string]: Draft }
	| { variant: number; value: Draft };

export interface FinalizeContext {
	unit: TimeUnit;
	locale: string;
}

export type Finalized =
	| { ok: true; value: { [key: string]: JsonValue } }
	| { ok: false; invalid: string[] };

const isRecord = (value: unknown): value is { [key: string]: unknown } =>
	typeof value === "object" && value !== null && !Array.isArray(value);

function matchesVariant(schema: Schema, value: JsonValue | undefined): boolean {
	if (value === undefined) return false;
	if (schema.type === "object") return isRecord(value);
	if (schema.type === "array") return Array.isArray(value);
	if (schema.type === "string") return typeof value === "string";
	if (schema.type === "boolean") return typeof value === "boolean";
	if (schema.type === "integer" || schema.type === "number")
		return typeof value === "number";
	return false;
}

// The draft of a value that came from outside (the registry example).
export function toDraft(
	schema: Schema,
	name: string,
	value: JsonValue | undefined,
): Draft {
	if (value === undefined) return undefined;
	const construct = classify(schema, name);
	switch (construct.kind) {
		case "json":
			return JSON.stringify(value, null, 2);
		case "object": {
			if (!isRecord(value)) return undefined;
			const draft: { [key: string]: Draft } = {};
			for (const field of construct.fields) {
				const sub = toDraft(
					field.schema,
					field.name,
					value[field.name] as JsonValue | undefined,
				);
				if (sub !== undefined) draft[field.name] = sub;
			}
			return draft;
		}
		case "list":
			return Array.isArray(value)
				? value.map((entry) =>
						toDraft(construct.items, construct.itemKey, entry as JsonValue),
					)
				: undefined;
		case "variants": {
			const index = Math.max(
				0,
				construct.variants.findIndex((variant) =>
					matchesVariant(variant, value),
				),
			);
			const variant = construct.variants[index];
			return variant === undefined
				? undefined
				: { variant: index, value: toDraft(variant, name, value) };
		}
		default:
			return value as Draft;
	}
}

interface Walk {
	context: FinalizeContext;
	invalid: string[];
}

// `undefined` means "leave the argument out".
function walk(
	walker: Walk,
	schema: Schema,
	name: string,
	draft: Draft,
	required: boolean,
	path: string,
): JsonValue | undefined {
	const construct = classify(schema, name);
	switch (construct.kind) {
		case "time": {
			if (typeof draft !== "string" || draft.trim() === "") return undefined;
			const result = normalizeTimeInput(
				localizeDecimalInput(draft, walker.context.locale),
				walker.context.unit,
			);
			if (result.ok) return result.value;
			walker.invalid.push(path);
			return undefined;
		}
		case "json": {
			if (typeof draft !== "string" || draft.trim() === "") return undefined;
			try {
				return JSON.parse(draft) as JsonValue;
			} catch {
				walker.invalid.push(path);
				return undefined;
			}
		}
		case "object": {
			if (!isRecord(draft)) return required ? {} : undefined;
			const out: { [key: string]: JsonValue } = {};
			for (const field of construct.fields) {
				const value = walk(
					walker,
					field.schema,
					field.name,
					(draft as { [key: string]: Draft })[field.name],
					field.required,
					`${path}.${field.name}`,
				);
				if (value !== undefined) out[field.name] = value;
			}
			return Object.keys(out).length === 0 && !required ? undefined : out;
		}
		case "list": {
			const rows = Array.isArray(draft) ? draft : [];
			const out: JsonValue[] = [];
			rows.forEach((row, index) => {
				const value = walk(
					walker,
					construct.items,
					construct.itemKey,
					row,
					false,
					`${path}[${index}]`,
				);
				// An empty text row is an unfilled row, not an empty value.
				if (value !== undefined && value !== "") out.push(value);
			});
			return out.length === 0 && !required ? undefined : out;
		}
		case "variants": {
			if (!isRecord(draft) || typeof draft.variant !== "number")
				return undefined;
			const variant = construct.variants[draft.variant];
			return variant === undefined
				? undefined
				: walk(walker, variant, name, draft.value as Draft, required, path);
		}
		case "number":
			return typeof draft === "number" && Number.isFinite(draft)
				? draft
				: undefined;
		case "boolean":
			return typeof draft === "boolean" ? draft : undefined;
		default:
			return typeof draft === "string" ? draft : undefined;
	}
}

// Draft to arguments. A field the person never touched is left out, a text
// field that was typed in and emptied is sent as an empty string (some
// commands clear a value that way), and an empty optional list or object is
// left out. Required arguments that are missing are the core's to report: its
// error carries a `path` that the form shows on the field.
export function finalize(
	schema: Schema,
	draft: Draft,
	context: FinalizeContext,
): Finalized {
	const walker: Walk = { context, invalid: [] };
	const value = walk(walker, schema, "", draft, true, "args");
	if (walker.invalid.length > 0) return { ok: false, invalid: walker.invalid };
	return {
		ok: true,
		value: (isRecord(value) ? value : {}) as { [key: string]: JsonValue },
	};
}
