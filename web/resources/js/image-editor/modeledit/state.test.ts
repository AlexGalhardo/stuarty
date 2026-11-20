import { beforeEach, describe, expect, test } from "vitest";

import type { Job, ModelView, ProviderView } from "../../editor/api/types";
import {
	capabilityOf,
	choicesOf,
	isFinal,
	modelJobStore,
	orderedModels,
	settledFor,
} from "./state";

const provider = (
	extra: Partial<ProviderView> & { id: string },
): ProviderView => ({
	name: extra.id,
	capabilities: [],
	credential_kinds: ["api_key"],
	default_endpoint: "",
	docs_url: "",
	connections: [],
	...extra,
});

const job = (extra: Partial<Job> & { job_id: string }): Job => ({
	kind: "image.edit",
	project_id: "prj_0000000001",
	state: "waiting_approval",
	progress: 0,
	actor: "local:ui",
	created_at: "2026-11-20T00:00:00Z",
	text: "",
	...extra,
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
	capability,
});

describe("capabilityOf", () => {
	test("a removal needs an editor of images and a generation a maker of images", () => {
		expect(capabilityOf("remove_background")).toBe("image_edit");
		expect(capabilityOf("generate")).toBe("image");
	});
});

describe("choicesOf", () => {
	const providers = [
		provider({
			id: "a",
			name: "A",
			capabilities: ["text", "image", "image_edit"],
			connections: ["con_1", "con_2"],
		}),
		provider({
			id: "b",
			name: "B",
			capabilities: ["text", "image"],
			connections: ["con_3"],
		}),
		provider({
			id: "c",
			name: "C",
			capabilities: ["text"],
			connections: ["con_4"],
		}),
	];

	test("lists the connections of the providers that have the capability of the operation", () => {
		expect(
			choicesOf(providers, "remove_background").map((c) => c.connectionId),
		).toEqual(["con_1", "con_2"]);
		expect(choicesOf(providers, "generate").map((c) => c.connectionId)).toEqual(
			["con_1", "con_2", "con_3"],
		);
	});

	test("names the provider, which the dialog says the picture is sent to", () => {
		expect(choicesOf(providers, "remove_background")[0]).toEqual({
			connectionId: "con_1",
			providerName: "A",
			label: "A (con_1)",
		});
	});

	test("lists nothing when no provider has it", () => {
		expect(choicesOf([providers[2] as ProviderView], "generate")).toEqual([]);
	});
});

describe("orderedModels", () => {
	test("puts the models hinted as image models first and keeps the rest, since the hint is not a rule", () => {
		const out = orderedModels([
			model("gpt-text", "text"),
			model("gpt-image-1", "image"),
			model("unknown"),
		]);
		expect(out.map((m) => m.id)).toEqual([
			"gpt-image-1",
			"gpt-text",
			"unknown",
		]);
	});
});

describe("the tracked job", () => {
	beforeEach(() => modelJobStore.getState().dismiss());

	test("is kept, updated by id and dismissed", () => {
		const state = modelJobStore.getState();
		state.track({
			job: job({ job_id: "job_1" }),
			operation: "remove_background",
			imageId: "img_1",
			layerId: "lyr_1",
			layerName: "Photo",
		});
		expect(modelJobStore.getState().tracked?.layerName).toBe("Photo");

		modelJobStore
			.getState()
			.update(job({ job_id: "job_1", state: "running", progress: 0.4 }));
		expect(modelJobStore.getState().tracked?.job.state).toBe("running");

		// the answer for another job is not the one on screen
		modelJobStore.getState().update(job({ job_id: "job_2", state: "failed" }));
		expect(modelJobStore.getState().tracked?.job.state).toBe("running");

		modelJobStore.getState().dismiss();
		expect(modelJobStore.getState().tracked).toBeNull();
	});

	test("settles only on the screen of the image the job edited", () => {
		const tracked = (state: Job["state"], applied: boolean) => ({
			job: job({
				job_id: "job_1",
				state,
				result: {
					generated: {
						provider: "openai",
						model: "gpt-image-1",
						items: [],
						requested: 1,
						delivered: 1,
						cost: { micros: 40_000, currency: "USD" },
						image: { image_id: "img_1", layer_id: "lyr_9", applied },
					},
				},
			}),
			operation: "generate" as const,
			imageId: "img_1",
			layerId: null,
			layerName: "",
		});

		expect(settledFor(tracked("succeeded", true), "img_1")).toEqual({
			select: "lyr_9",
		});
		expect(settledFor(tracked("succeeded", false), "img_1")).toEqual({
			select: null,
		});
		// another image opened meanwhile is not reloaded, and a job not done settles nothing
		expect(settledFor(tracked("succeeded", true), "img_2")).toBeNull();
		expect(settledFor(tracked("running", true), "img_1")).toBeNull();
		expect(settledFor(null, "img_1")).toBeNull();
	});

	test("isFinal is true for the three states that end a job", () => {
		expect(isFinal(job({ job_id: "j", state: "succeeded" }))).toBe(true);
		expect(isFinal(job({ job_id: "j", state: "failed" }))).toBe(true);
		expect(isFinal(job({ job_id: "j", state: "cancelled" }))).toBe(true);
		expect(isFinal(job({ job_id: "j", state: "running" }))).toBe(false);
		expect(isFinal(job({ job_id: "j", state: "waiting_approval" }))).toBe(
			false,
		);
	});
});
