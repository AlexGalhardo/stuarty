import { describe, expect, it } from "vitest";
import {
	DecodeClient,
	type DecoderPreference,
	decoderPreference,
	type WorkerLike,
} from "./client";
import type { FromWorker, ToWorker } from "./protocol";

class FakeFrame {
	closed = false;
	constructor(readonly timestamp: number) {}
	close(): void {
		this.closed = true;
	}
}

class FakeWorker implements WorkerLike<FakeFrame> {
	sent: ToWorker[] = [];
	terminated = false;
	onmessage: ((event: { data: FromWorker<FakeFrame> }) => void) | null = null;
	postMessage(message: ToWorker): void {
		this.sent.push(message);
	}
	terminate(): void {
		this.terminated = true;
	}
	reply(message: FromWorker<FakeFrame>): void {
		this.onmessage?.({ data: message });
	}
	frames(): Extract<ToWorker, { type: "frame" }>[] {
		return this.sent.filter(
			(m): m is Extract<ToWorker, { type: "frame" }> => m.type === "frame",
		);
	}
}

function setup(preference: DecoderPreference = "auto", withWorker = true) {
	const worker = new FakeWorker();
	let changes = 0;
	let clock = 0;
	const client = new DecodeClient<FakeFrame>({
		preference,
		createWorker: () => (withWorker ? worker : null),
		indexUrl: (a) => `/chunks/${a}`,
		proxyUrl: (a) => `/proxy/${a}`,
		onChange: () => {
			changes++;
		},
		now: () => clock,
		cacheSize: 3,
		idleMs: 1000,
	});
	return {
		worker,
		client,
		changes: () => changes,
		tick: (ms: number) => {
			clock += ms;
		},
	};
}

function frameReply(
	asset: string,
	id: number,
	sample: number,
	open = 0,
): FromWorker<FakeFrame> {
	return {
		type: "frame",
		id,
		asset,
		sample,
		timestamp: sample * 1000,
		lo: sample * 1000 - 500,
		hi: sample * 1000 + 500,
		frame: new FakeFrame(sample * 1000),
		open,
	};
}

describe("decoderPreference", () => {
	it("reads the query parameter", () => {
		expect(decoderPreference("")).toBe("auto");
		expect(decoderPreference("?decoder=video")).toBe("video");
		expect(decoderPreference("?decoder=webcodecs")).toBe("webcodecs");
		expect(decoderPreference("?decoder=other")).toBe("auto");
	});
});

describe("DecodeClient", () => {
	it("falls back for every asset when forced to video", () => {
		const { client, worker } = setup("video");
		expect(client.enabled).toBe(false);
		expect(client.request("a", 0, "seek")).toEqual({ state: "fallback" });
		expect(worker.sent).toEqual([]);
	});

	it("falls back when the browser has no worker or no WebCodecs", () => {
		const { client } = setup("auto", false);
		expect(client.request("a", 0, "seek")).toEqual({ state: "fallback" });
	});

	it("opens an asset once and is pending until it is ready", () => {
		const { client, worker } = setup();
		expect(client.request("a", 0, "seek")).toEqual({ state: "pending" });
		expect(client.request("a", 0, "seek")).toEqual({ state: "pending" });
		expect(worker.sent).toEqual([
			{ type: "open", asset: "a", indexUrl: "/chunks/a", proxyUrl: "/proxy/a" },
		]);
	});

	it("asks for the frame once ready and returns it when it arrives", () => {
		const { client, worker, changes } = setup();
		client.request("a", 2000, "seek");
		worker.reply({ type: "opened", asset: "a", open: 0 });
		expect(changes()).toBe(1);
		expect(client.request("a", 2000, "seek")).toEqual({ state: "pending" });
		const [ask] = worker.frames();
		expect(ask).toMatchObject({ asset: "a", micros: 2000, mode: "seek" });
		worker.reply(frameReply("a", ask?.id ?? 0, 2));
		const hit = client.request("a", 2100, "seek");
		expect(hit).toMatchObject({ state: "frame", exact: true, timestamp: 2000 });
	});

	it("keeps one request in flight and sends only the latest time", () => {
		const { client, worker } = setup();
		client.request("a", 0, "seek");
		worker.reply({ type: "opened", asset: "a", open: 0 });
		client.request("a", 1000, "seek");
		client.request("a", 5000, "seek");
		client.request("a", 9000, "seek");
		expect(worker.frames()).toHaveLength(1);
		worker.reply(frameReply("a", worker.frames()[0]?.id ?? 0, 1));
		const asks = worker.frames();
		expect(asks).toHaveLength(2);
		expect(asks[1]?.micros).toBe(9000);
	});

	it("sends the latest time again when the worker skipped the request", () => {
		const { client, worker } = setup();
		client.request("a", 0, "seek");
		worker.reply({ type: "opened", asset: "a", open: 0 });
		client.request("a", 1000, "seek");
		client.request("a", 7000, "seek");
		worker.reply({
			type: "skipped",
			id: worker.frames()[0]?.id ?? 0,
			asset: "a",
			open: 0,
		});
		expect(worker.frames()[1]?.micros).toBe(7000);
	});

	it("shows the last frame, not an exact one, while the right one is decoded", () => {
		const { client, worker } = setup();
		client.request("a", 0, "seek");
		worker.reply({ type: "opened", asset: "a", open: 0 });
		client.request("a", 1000, "seek");
		worker.reply(frameReply("a", worker.frames()[0]?.id ?? 0, 1));
		const stale = client.request("a", 9000, "seek");
		expect(stale).toMatchObject({ state: "frame", exact: false });
	});

	it("asks for the next frame too while playing", () => {
		const { client, worker } = setup();
		client.request("a", 0, "play", 1000);
		worker.reply({ type: "opened", asset: "a", open: 0 });
		client.request("a", 1000, "play", 1000);
		worker.reply(frameReply("a", worker.frames()[0]?.id ?? 0, 1));
		client.request("a", 1000, "play", 1000);
		expect(worker.frames().at(-1)?.micros).toBe(2000);
		expect(worker.frames().at(-1)?.mode).toBe("play");
	});

	it("falls back per asset and drops the frames it held", () => {
		const { client, worker } = setup();
		client.request("a", 0, "seek");
		client.request("b", 0, "seek");
		worker.reply({ type: "opened", asset: "a", open: 0 });
		worker.reply({ type: "opened", asset: "b", open: 0 });
		client.request("a", 1000, "seek");
		const reply = frameReply("a", worker.frames()[0]?.id ?? 0, 1);
		worker.reply(reply);
		worker.reply({ type: "fallback", asset: "a", reason: "404", open: 0 });
		client.sweep();
		expect(client.request("a", 1000, "seek")).toEqual({ state: "fallback" });
		expect(client.request("b", 0, "seek").state).not.toBe("fallback");
		expect(client.handles("a")).toBe(false);
		expect(client.handles("b")).toBe(true);
		expect(client.stats.fallbackAssets).toBe(1);
		if (reply.type === "frame") expect(reply.frame.closed).toBe(true);
	});

	it("closes a frame that arrives for an asset that was dropped", () => {
		const { client, worker, tick } = setup();
		client.request("a", 0, "seek");
		worker.reply({ type: "opened", asset: "a", open: 0 });
		client.request("a", 1000, "seek");
		tick(2000);
		client.sweep();
		expect(worker.sent.at(-1)).toEqual({ type: "drop", asset: "a" });
		const late = frameReply("a", worker.frames()[0]?.id ?? 0, 1);
		worker.reply(late);
		if (late.type === "frame") expect(late.frame.closed).toBe(true);
		expect(client.stats.openFrames).toBe(0);
	});

	it("keeps the open frames at the cache bound plus what the worker holds", () => {
		const { client, worker } = setup();
		client.request("a", 0, "seek");
		worker.reply({ type: "opened", asset: "a", open: 0 });
		const frames: FakeFrame[] = [];
		for (let i = 0; i < 200; i++) {
			client.request("a", i * 1000, "seek");
			const ask = worker.frames().at(-1);
			const reply = frameReply("a", ask?.id ?? 0, i, 2);
			if (reply.type === "frame") frames.push(reply.frame);
			worker.reply(reply);
			client.sweep();
			expect(client.stats.openFrames).toBeLessThanOrEqual(3 + 2);
		}
		expect(client.stats.peakOpenFrames).toBeLessThanOrEqual(3 + 2 + 1);
		// Every frame but the cached ones was closed.
		expect(frames.filter((f) => !f.closed)).toHaveLength(3);
	});

	it("closes everything and ends the worker on dispose", () => {
		const { client, worker } = setup();
		client.request("a", 0, "seek");
		worker.reply({ type: "opened", asset: "a", open: 0 });
		client.request("a", 1000, "seek");
		const reply = frameReply("a", worker.frames()[0]?.id ?? 0, 1);
		worker.reply(reply);
		client.dispose();
		expect(worker.terminated).toBe(true);
		if (reply.type === "frame") expect(reply.frame.closed).toBe(true);
	});
});
