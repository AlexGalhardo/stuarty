import { describe, expect, test } from "vitest";

import {
	compareCatalogs,
	flatten,
	placeholders,
	untranslated,
} from "./i18n-catalog";

describe("flatten", () => {
	test("nests with dots and keeps a plural object as one leaf", () => {
		const leaves = flatten({
			a: { b: "x {n}", c: { one: "1 {count}", other: "{count}" } },
		});
		expect([...leaves.keys()]).toEqual(["a.b", "a.c"]);
		expect(leaves.get("a.c")?.categories).toEqual(["one", "other"]);
	});

	test("placeholders are the names in braces", () => {
		expect([...placeholders(["{a} and {b}", "{a}"])].sort()).toEqual(["a", "b"]);
	});
});

describe("compareCatalogs", () => {
	const en = flatten({ k: "Hello {name}", n: { one: "{count} job", other: "{count} jobs" } });

	test("I1: a key in only one locale fails, both ways", () => {
		const pt = flatten({ k: "Olá {name}", extra: "x" });
		const messages = compareCatalogs(en, pt, "pt-BR").map((p) => p.message);
		expect(messages).toContain("n is missing in pt-BR");
		expect(messages).toContain("extra is only in pt-BR");
	});

	test("I2: placeholders must match, plurals may differ in categories", () => {
		const bad = flatten({ k: "Olá {nome}", n: { one: "{count} tarefa", many: "{count} de tarefas", other: "{count} tarefas" } });
		const problems = compareCatalogs(en, bad, "pt-BR");
		expect(problems).toHaveLength(1);
		expect(problems[0]?.rule).toBe("I2");
		expect(problems[0]?.message).toContain("k has placeholders {name}");
	});

	test("a plural in one locale and a string in the other fails", () => {
		const pt = flatten({ k: "Olá {name}", n: "{count} tarefas" });
		expect(compareCatalogs(en, pt, "pt-BR").map((p) => p.rule)).toEqual(["I2"]);
	});

	test("equal catalogues pass", () => {
		const pt = flatten({ k: "Olá {name}", n: { one: "{count} tarefa", other: "{count} tarefas" } });
		expect(compareCatalogs(en, pt, "pt-BR")).toEqual([]);
	});
});

describe("untranslated (I7)", () => {
	const en = flatten({ title: "Export", name: "Vukem", n: { one: "1 job", other: "{count} jobs" } });

	test("fails a key whose text equals the English one", () => {
		const pt = flatten({ title: "Export", name: "Vukem", n: { one: "1 tarefa", other: "{count} tarefas" } });
		expect(untranslated(en, pt, "pt-BR", new Set(["name"])).map((p) => p.message)).toEqual([
			"title has the English text in pt-BR",
		]);
	});

	test("passes when the same text is on the allowlist or translated", () => {
		const pt = flatten({ title: "Exportar", name: "Vukem", n: { one: "1 tarefa", other: "{count} tarefas" } });
		expect(untranslated(en, pt, "pt-BR", new Set(["name"]))).toEqual([]);
	});
});
