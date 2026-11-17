import { describe, expect, test, vi } from "vitest";

import type { TemplateResult, TemplateSummary } from "../../editor/api/types";
import { importOutcome } from "./Templates";

vi.mock("@inertiajs/react", () => ({ Head: () => null }));

const summary = (name: string): TemplateSummary =>
	({ template_id: "tpl_1", name }) as TemplateSummary;

const result = (name: string, deduplicated?: boolean): TemplateResult => ({
	template: summary(name),
	text: "",
	...(deduplicated === undefined ? {} : { deduplicated }),
});

describe("what a template import brought (VV-128)", () => {
	test("the service says the same file was already stored", () => {
		expect(
			importOutcome(result("Quote card", true), [summary("Quote card")]),
		).toBe("same_file");
	});

	test("the same name with other content is a second template", () => {
		expect(importOutcome(result("Quote card"), [summary("Quote card")])).toBe(
			"same_name",
		);
	});

	test("a new name is a new template", () => {
		expect(importOutcome(result("Quote card"), [summary("Other")])).toBe("new");
	});
});
