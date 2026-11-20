import { describe, expect, test } from "vitest";

import type { ModelView, ProviderView } from "../editor/api/types";
import { anyMaker, choicesOf, orderedModels } from "./GenerateDialog";

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
	test("names a connection as the person named it and never by its id", () => {
		const providers = [
			provider("openai", ["image"], ["con_o1", "con_o2", "con_o3"]),
		];
		const labels = choicesOf(providers, "image", {
			con_o1: "openai",
			con_o2: "Work key",
		}).map((c) => c.label);
		expect(labels).toEqual(["openai 1", "openai (Work key)", "openai 2"]);
		for (const label of labels) expect(label).not.toContain("con_");
	});

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

	test("offers the video tab only the connections of a provider that makes video, and puts its models first (epic 11)", () => {
		const providers = [
			provider("openai", ["text", "image", "voice"], ["con_o1"]),
			provider("clips", ["video"], ["con_c1"]),
		];
		expect(choicesOf(providers, "video").map((c) => c.connectionId)).toEqual([
			"con_c1",
		]);
		expect(
			choicesOf([provider("openai", ["text", "image"], ["con_o1"])], "video"),
		).toEqual([]);

		const models = [model("gpt-test", "text"), model("clip-1", "video")];
		expect(orderedModels(models, "video").map((m) => m.id)).toEqual([
			"clip-1",
			"gpt-test",
		]);
	});

	test("tells apart no provider that makes video from one that is not connected (epic 11)", () => {
		// No provider of the product makes video: there is nothing to connect, so no link to Connections.
		const today = [
			provider("openai", ["text", "image", "voice"], ["con_o1"]),
			provider("anthropic", ["text"], []),
		];
		expect(anyMaker(today, "video")).toBe(false);
		expect(choicesOf(today, "video")).toEqual([]);

		// A provider makes video and has no connection yet: the person is sent to connect one.
		const later = [...today, provider("clips", ["video"], [])];
		expect(anyMaker(later, "video")).toBe(true);
		expect(choicesOf(later, "video")).toEqual([]);
	});
});
