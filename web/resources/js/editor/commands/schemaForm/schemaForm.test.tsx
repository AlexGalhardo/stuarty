import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import commands from "@/generated/commands.json";
import { I18nProvider } from "@/lib/i18n";
import type { CommandDetail } from "../../api/types";
import { devFixture } from "../../boot/devFixture";
import { startHarness } from "../../fixtures/harness";
import { EditorProvider } from "../../state/EditorContext";
import { classify, fieldsOf, type Schema } from "./classify";
import { type Draft, finalize, toDraft } from "./draft";
import { resolvesPath, SchemaForm } from "./SchemaForm";

const registry = commands as unknown as CommandDetail[];
const context = { unit: "s" as const, locale: "en" };

const obj = (properties: Record<string, Schema>, required: string[] = []) => ({
	type: "object",
	properties,
	required,
	additionalProperties: false,
});

describe("classify", () => {
	test("each construct of the table of SDD-012 9.2", () => {
		const kind = (schema: Schema, name = "x"): string =>
			classify(schema, name).kind;
		expect(kind({ type: "string" }, "item_id")).toBe("id");
		expect(kind({ type: "string", pattern: "^trk_" }, "where")).toBe("id");
		expect(kind({ type: "string" }, "new_id")).toBe("text");
		expect(kind({ type: "string", "x-vv-kind": "time" }, "when")).toBe("time");
		expect(kind({ type: "string", description: "A length" })).toBe("text");
		expect(kind({ type: "string" }, "start")).toBe("text");
		expect(kind({ type: ["string", "null"], "x-vv-kind": "time" }, "to")).toBe(
			"time",
		);
		expect(kind({ type: "string", enum: ["a", "b"] })).toBe("enum");
		expect(kind({ type: "string" }, "name")).toBe("text");
		expect(kind({ type: "number", minimum: 0, maximum: 1 })).toBe("number");
		expect(kind({ type: "integer" })).toBe("number");
		expect(kind({ type: "boolean" })).toBe("boolean");
		expect(kind({ type: "array", items: { type: "string" } })).toBe("list");
		expect(kind({ type: "array", items: obj({ a: { type: "string" } }) })).toBe(
			"list",
		);
		expect(kind(obj({ a: { type: "string" } }))).toBe("object");
		expect(kind({ oneOf: [{ type: "string" }, { type: "number" }] })).toBe(
			"variants",
		);
		expect(kind({ anyOf: [{ type: "string" }] })).toBe("variants");
		expect(kind({})).toBe("json");
		expect(kind({ type: "object", additionalProperties: {} })).toBe("json");
		expect(kind({ type: "array", items: { type: "array" } })).toBe("json");
	});

	test("number bounds come from the schema", () => {
		expect(classify({ type: "integer", minimum: 1, maximum: 60 }, "n")).toEqual(
			{ kind: "number", integer: true, min: 1, max: 60 },
		);
	});

	test("only a map keyed by names falls back to JSON", () => {
		const fallbacks: string[] = [];
		const walk = (schema: Schema, name: string, path: string): void => {
			const construct = classify(schema, name);
			if (construct.kind === "json") fallbacks.push(path);
			if (construct.kind === "object")
				for (const f of construct.fields)
					walk(f.schema, f.name, `${path}.${f.name}`);
			if (construct.kind === "list")
				walk(construct.items, construct.itemKey, `${path}[]`);
		};
		for (const command of registry) {
			if (command.internal === true) continue;
			for (const field of fieldsOf(command.args_schema))
				walk(field.schema, field.name, `${command.name}.${field.name}`);
		}
		expect(fallbacks.sort()).toEqual([
			"audio.processor_add.params",
			"audio.processor_set.params",
			"template.set_info.colors",
		]);
	});
});

describe("finalize", () => {
	const schema = obj(
		{
			item_id: { type: "string" },
			start: { type: "string", "x-vv-kind": "time" },
			volume: { type: "number" },
			mute: { type: "boolean" },
			name: { type: "string" },
			tags: { type: "array", items: { type: "string" } },
			words: {
				type: "array",
				items: obj({
					text: { type: "string" },
					start: { type: "string", "x-vv-kind": "time" },
				}),
			},
			extra: { type: "object", additionalProperties: {} },
		},
		["item_id"],
	);

	test("a time gets its unit, an untouched field is left out", () => {
		const draft: Draft = { item_id: "itm_a", start: "1.5", name: "" };
		expect(finalize(schema, draft, context)).toEqual({
			ok: true,
			value: { item_id: "itm_a", start: "1.5s", name: "" },
		});
	});

	test("the default unit and the pt-BR comma apply to times", () => {
		expect(
			finalize(schema, { start: "12" }, { unit: "f", locale: "en" }),
		).toMatchObject({ value: { start: "12f" } });
		expect(
			finalize(schema, { start: "12,5" }, { unit: "s", locale: "pt-BR" }),
		).toMatchObject({ value: { start: "12.5s" } });
	});

	test("an invalid time or JSON blocks the send and names the path", () => {
		expect(finalize(schema, { start: "abc" }, context)).toEqual({
			ok: false,
			invalid: ["args.start"],
		});
		expect(
			finalize(schema, { words: [{ start: "1:2:3:4" }], extra: "{" }, context),
		).toEqual({ ok: false, invalid: ["args.words[0].start", "args.extra"] });
	});

	test("a removed nullable field is sent as null and a plain one is not", () => {
		const nullable = obj({
			item_id: { type: "string" },
			to: { type: ["string", "null"], "x-vv-kind": "time" },
			name: { type: "string" },
		});
		expect(
			finalize(nullable, { item_id: "itm_a", to: null, name: null }, context),
		).toEqual({ ok: true, value: { item_id: "itm_a", to: null } });
		expect(toDraft(nullable, "", { to: null })).toEqual({ to: null });
	});

	test("lists drop empty rows and are omitted when empty and optional", () => {
		expect(
			finalize(
				schema,
				{ tags: ["a", "", undefined], words: [{ text: "hi", start: "2" }, {}] },
				context,
			),
		).toEqual({
			ok: true,
			value: { tags: ["a"], words: [{ text: "hi", start: "2s" }] },
		});
		expect(finalize(schema, { tags: [] }, context)).toEqual({
			ok: true,
			value: {},
		});
	});

	test("an untouched required boolean is sent as its default or false (VV-178)", () => {
		const flags = obj(
			{
				enabled: { type: "boolean" },
				on: { type: "boolean", default: true },
				optional: { type: "boolean" },
			},
			["enabled", "on"],
		);
		expect(finalize(flags, {}, context)).toEqual({
			ok: true,
			value: { enabled: false, on: true },
		});
		expect(finalize(flags, { enabled: true, on: false }, context)).toEqual({
			ok: true,
			value: { enabled: true, on: false },
		});
	});

	test("a JSON fallback is parsed", () => {
		expect(finalize(schema, { extra: '{"a": 1}' }, context)).toEqual({
			ok: true,
			value: { extra: { a: 1 } },
		});
	});

	test("a oneOf sends the chosen variant only", () => {
		const choice = obj({
			value: { oneOf: [{ type: "string" }, { type: "number" }] },
		});
		expect(
			finalize(choice, { value: { variant: 1, value: 3 } }, context),
		).toEqual({ ok: true, value: { value: 3 } });
		expect(finalize(choice, { value: undefined }, context)).toEqual({
			ok: true,
			value: {},
		});
		expect(toDraft(choice, "", { value: 3 })).toEqual({
			value: { variant: 1, value: 3 },
		});
	});
});

describe("the registry examples", () => {
	test("every example survives the form unchanged", () => {
		for (const command of registry) {
			if (command.internal === true) continue;
			const draft = toDraft(
				command.args_schema,
				"",
				command.example as Parameters<typeof toDraft>[2],
			);
			const result = finalize(command.args_schema, draft, context);
			expect(result, command.name).toEqual({
				ok: true,
				value: command.example,
			});
		}
	});
});

describe("resolvesPath", () => {
	const schema = obj({
		start: { type: "string" },
		words: { type: "array", items: obj({ text: { type: "string" } }) },
	});
	test("a path lands on a field of the form or does not", () => {
		expect(resolvesPath(schema, "args.start")).toBe(true);
		expect(resolvesPath(schema, "args.words[2].text")).toBe(true);
		expect(resolvesPath(schema, "args.words[0].nope")).toBe(false);
		expect(resolvesPath(schema, "args.zzz")).toBe(false);
		expect(resolvesPath(schema, "ops[0]")).toBe(false);
	});
});

describe("SchemaForm", () => {
	test("renders a control for every property, with required marks and ids", async () => {
		const h = await startHarness();
		const move = registry.find((c) => c.name === "item.move");
		if (move === undefined) throw new Error("item.move is not in the registry");
		const html = renderToString(
			<I18nProvider messages={devFixture(1).bootstrap.messages} locale="en">
				<EditorProvider runtime={h.runtime}>
					<SchemaForm
						command={move}
						value={{ item_id: "itm_a", start: "2s" }}
						revision={0}
						onChange={() => undefined}
						error={null}
					/>
				</EditorProvider>
			</I18nProvider>,
		);
		for (const name of ["item_id", "start", "track_id"])
			expect(html).toContain(`data-path="args.${name}"`);
		expect(html).toContain('value="itm_a"');
		expect(html).toContain('value="2s"');
		expect(html).toMatch(/\(<!-- -->required<!-- -->\)/);
		expect(html).not.toContain("data-fallback");
		expect(html).toMatch(/<datalist/);
	});

	test("a nullable field has a Remove toggle that is checked for null", async () => {
		const h = await startHarness();
		const command = {
			...(registry[0] as CommandDetail),
			args_schema: obj({
				to: { type: ["string", "null"], "x-vv-kind": "time" },
			}),
		} as CommandDetail;
		const render = (value: Record<string, unknown>): string =>
			renderToString(
				<I18nProvider messages={devFixture(1).bootstrap.messages} locale="en">
					<EditorProvider runtime={h.runtime}>
						<SchemaForm
							command={command}
							value={value as never}
							revision={0}
							onChange={() => undefined}
							error={null}
						/>
					</EditorProvider>
				</I18nProvider>,
			);
		const open = render({});
		expect(open).toContain('data-path="args.to.remove"');
		expect(open).toContain('data-path="args.to"');
		const removed = render({ to: null });
		expect(removed).toContain('data-path="args.to.remove"');
		expect(removed).not.toContain('data-path="args.to"');
	});
});
