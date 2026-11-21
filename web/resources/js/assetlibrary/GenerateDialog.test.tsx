import { describe, expect, test } from "vitest";

import type { Transport } from "../editor/api/transport";
import type { ModelView, ProviderView } from "../editor/api/types";
import {
	anyMaker,
	choicesOf,
	firstFrames,
	libraryOf,
	orderedModels,
} from "./GenerateDialog";

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

describe("the library the first frame is listed from (VV-334)", () => {
	test("is the first active timeline of a production that has two, never the production id", () => {
		const items = [
			{ id: "itm_1", kind: "brief", state: "active" },
			{ id: "itm_2", kind: "timeline", state: "trashed", timeline_id: "tl_0" },
			{ id: "itm_3", kind: "timeline", state: "active", timeline_id: "tl_1" },
			{ id: "itm_4", kind: "timeline", state: "active", timeline_id: "tl_2" },
		];
		expect(libraryOf(items)).toBe("tl_1");
	});

	test("is none for a production with no timeline, whose images only the library lists", () => {
		expect(
			libraryOf([{ id: "itm_1", kind: "brief", state: "active" }]),
		).toBeNull();
	});
});

describe("the first frames of a production (VV-334)", () => {
	const timelineItem = (timeline_id: string, state = "active") => ({
		id: `pit_${timeline_id}`,
		kind: "timeline",
		title: timeline_id,
		state,
		timeline_id,
	});
	const media = (asset_id: string, extra: object = {}) => ({
		asset_id,
		kind: "image",
		state: "ready",
		trashed: false,
		...extra,
	});

	// A server of the requests the dialog makes: it answers by path and records them.
	function server(answers: Record<string, unknown>): {
		transport: Transport;
		paths: string[];
	} {
		const paths: string[] = [];
		return {
			paths,
			transport: {
				send: async ({ path }) => {
					paths.push(path);
					const body = answers[path];
					if (body === undefined) throw new Error(`unexpected ${path}`);
					return { status: 200, body };
				},
				openStream: () => ({ close: () => {} }),
			},
		};
	}

	test("with two timelines the first is named, since it has the id of the production", async () => {
		const { transport, paths } = server({
			"/projects/prj_x/describe": {
				items: [timelineItem("prj_x"), timelineItem("prj_y")],
			},
			"/projects/prj_x/media?timeline_id=prj_x": {
				assets: [
					media("ast_ready"),
					media("ast_video", { kind: "video" }),
					media("ast_busy", { state: "pending" }),
					media("ast_gone", { trashed: true }),
				],
			},
		});
		expect(await firstFrames(transport, "prj_x")).toEqual(["ast_ready"]);
		expect(paths).toContain("/projects/prj_x/media?timeline_id=prj_x");
	});

	test("with the first in the trash, the next one is the library", async () => {
		const { transport } = server({
			"/projects/prj_x/describe": {
				items: [timelineItem("prj_x", "trashed"), timelineItem("prj_y")],
			},
			"/projects/prj_y/media?timeline_id=prj_y": {
				assets: [media("ast_second")],
			},
		});
		expect(await firstFrames(transport, "prj_x")).toEqual(["ast_second"]);
	});

	test("with no timeline they come from the library, by the id each has in this production", async () => {
		const { transport, paths } = server({
			"/projects/prj_x/describe": { items: [] },
			"/assets?kind=image&project=prj_x": {
				assets: [
					{
						hash: "sha256:a",
						poster: { timeline_id: "prj_x", asset_id: "ast_here" },
					},
					{
						hash: "sha256:b",
						poster: { timeline_id: "prj_a", asset_id: "ast_of_another" },
					},
				],
			},
			"/assets/sha256%3Ab": {
				uses: [
					{
						project_id: "prj_a",
						timelines: ["prj_a"],
						asset_id: "ast_of_another",
					},
					{ project_id: "prj_x", timelines: ["prj_x"], asset_id: "ast_shared" },
				],
			},
		});
		expect(await firstFrames(transport, "prj_x")).toEqual([
			"ast_here",
			"ast_shared",
		]);
		expect(paths.some((path) => path.includes("/media"))).toBe(false);
	});
});
