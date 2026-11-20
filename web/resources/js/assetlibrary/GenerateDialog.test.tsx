import { describe, expect, test } from "vitest";

import type { ModelView, ProviderView } from "../editor/api/types";
import { choicesOf, orderedModels } from "./GenerateDialog";

const provider = (
	id: string,
	capabilities: string[],
	connections: string[],
): ProviderView => ({
	id,
	name: id,
	capabilities,
	credential_kinds: ["api_key"],
	default_endpoint: "https://example.test/v1",
	docs_url: "https://example.test/keys",
	connections,
});

const model = (
	id: string,
	capability?: ModelView["capability"],
): ModelView => ({
	id,
	name: id,
	context_tokens: 0,
	output_tokens: 0,
	price: null,
	price_source: "none",
	...(capability === undefined ? {} : { capability }),
});

describe("the choices of the Generate dialog", () => {
	test("offers the connections of the providers that have the capability", () => {
		const providers = [
			provider("anthropic", ["text"], ["con_a"]),
			provider("openai", ["text", "image", "voice"], ["con_o1", "con_o2"]),
		];
		expect(choicesOf(providers, "image").map((c) => c.connectionId)).toEqual([
			"con_o1",
			"con_o2",
		]);
		expect(choicesOf(providers, "voice")).toHaveLength(2);
		expect(
			choicesOf([provider("anthropic", ["text"], ["con_a"])], "image"),
		).toEqual([]);
	});

	test("puts the models hinted for the capability first and keeps the rest, since a hint is not a rule", () => {
		const models = [
			model("gpt-test", "text"),
			model("tts-test", "voice"),
			model("gpt-image-test", "image"),
			model("whisper-test"),
		];
		expect(orderedModels(models, "image").map((m) => m.id)).toEqual([
			"gpt-image-test",
			"gpt-test",
			"tts-test",
			"whisper-test",
		]);
		expect(orderedModels(models, "voice")[0]?.id).toBe("tts-test");
	});
});
