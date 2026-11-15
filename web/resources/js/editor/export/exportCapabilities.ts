import type { ExportCapabilities } from "../api/types";

type Json = { [key: string]: unknown };

const NONE: ExportCapabilities = { formats: [] };

function asObject(value: unknown): Json | undefined {
	return typeof value === "object" && value !== null && !Array.isArray(value)
		? (value as Json)
		: undefined;
}

function resolve(doc: Json, schema: unknown): Json | undefined {
	const node = asObject(schema);
	const ref = node?.$ref;
	if (typeof ref !== "string" || !ref.startsWith("#/")) return node;
	let target: unknown = doc;
	for (const part of ref.slice(2).split("/")) {
		target = asObject(target)?.[part];
	}
	return asObject(target);
}

// The values a member lists as an enum; a member with no enum is offered with
// every format the product knows, and the core refuses one with its own hint
// when the encoder cannot.
function choices(schema: Json | undefined, fallback: unknown[]): unknown[] {
	if (schema === undefined) return [];
	const values = schema.enum;
	return Array.isArray(values) ? values : fallback;
}

// What the server accepts as export arguments, read from the `args` member of
// the job_start request in its OpenAPI document (SDD-003 section 2.8). A
// member the schema does not list is not offered, so the dialog degrades to
// the plain video export on a server that has not wired the format yet; the
// browser never guesses what the core supports.
export function exportCapabilities(openapi: unknown): ExportCapabilities {
	const doc = asObject(openapi);
	if (doc === undefined) return NONE;
	const paths = asObject(doc.paths);
	if (paths === undefined) return NONE;
	const key = Object.keys(paths).find((path) =>
		path.endsWith("/projects/{id}/jobs"),
	);
	const post = asObject(asObject(paths[key ?? ""])?.post);
	const content = asObject(
		asObject(asObject(post?.requestBody)?.content)?.["application/json"],
	);
	const body = resolve(doc, content?.schema);
	const args = resolve(doc, asObject(body?.properties)?.args);
	const props = asObject(args?.properties);
	if (props === undefined) return NONE;
	const member = (name: string): Json | undefined => resolve(doc, props[name]);
	return {
		formats: choices(member("format"), ["mp4", "gif", "wav", "aac"]).filter(
			(value): value is string => typeof value === "string",
		),
	};
}
