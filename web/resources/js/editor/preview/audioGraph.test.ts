import { describe, expect, test } from "vitest";

import { AudioGraph } from "./audioGraph";

function fakeContext() {
	const master = { gain: { value: 1 }, connect() {}, disconnect() {} };
	const context = {
		destination: {},
		createGain: () => master,
	} as unknown as AudioContext;
	return { context, master };
}

describe("AudioGraph", () => {
	test("has no input until it is attached to a context", () => {
		expect(new AudioGraph().input).toBeNull();
	});

	test("a level set before the context exists is applied when it attaches", () => {
		const graph = new AudioGraph();
		graph.setLevel(0.4, false);
		const { context, master } = fakeContext();
		graph.attach(context);
		expect(master.gain.value).toBe(0.4);
	});

	test("mute silences the master and unmute restores the volume", () => {
		const graph = new AudioGraph();
		const { context, master } = fakeContext();
		graph.attach(context);
		graph.setLevel(0.7, true);
		expect(master.gain.value).toBe(0);
		graph.setLevel(0.7, false);
		expect(master.gain.value).toBe(0.7);
	});

	test("the volume is clamped to 0 to 1", () => {
		const graph = new AudioGraph();
		const { context, master } = fakeContext();
		graph.attach(context);
		graph.setLevel(5, false);
		expect(master.gain.value).toBe(1);
	});
});
