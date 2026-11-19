import { describe, expect, it } from "vitest";

import {
	RenderClient,
	type RenderFailure,
	type RenderWorkerLike,
} from "./client";
import type { FromRenderWorker, RenderInput, ToRenderWorker } from "./protocol";

type Bitmap = { name: string; closed: boolean };
type Src = { close(): void };

class FakeWorker implements RenderWorkerLike<Src, Bitmap> {
	sent: ToRenderWorker<Src>[] = [];
	onmessage: ((event: { data: FromRenderWorker<Bitmap> }) => void) | null =
		null;
	terminated = false;
	postMessage(message: ToRenderWorker<Src>): void {
		this.sent.push(message);
	}
	terminate(): void {
		this.terminated = true;
	}
	say(message: FromRenderWorker<Bitmap>): void {
		this.onmessage?.({ data: message });
	}
	renders(): number[] {
		return this.sent.flatMap((m) => (m.type === "render" ? [m.seq] : []));
	}
}

const input = (n: number): RenderInput => ({
	version: 1,
	output: { width: n, height: 1, scale: 1 },
	plan: {},
});

function setup(): {
	worker: FakeWorker;
	client: RenderClient<Src, Bitmap>;
	frames: number[];
	failures: RenderFailure[];
	closed: Bitmap[];
} {
	const worker = new FakeWorker();
	const frames: number[] = [];
	const failures: RenderFailure[] = [];
	const closed: Bitmap[] = [];
	const client = new RenderClient<Src, Bitmap>(
		worker,
		{
			onFrame: (_b, seq) => void frames.push(seq),
			onError: (f) => void failures.push(f),
		},
		(b) => {
			b.closed = true;
			closed.push(b);
		},
	);
	return { worker, client, frames, failures, closed };
}

const frame = (seq: number): FromRenderWorker<Bitmap> => ({
	type: "frame",
	seq,
	bitmap: { name: `f${seq}`, closed: false },
	open: 0,
});

describe("RenderClient", () => {
	it("posts one render at a time and keeps only the newest while waiting", () => {
		const { worker, client, frames } = setup();
		client.render(input(1));
		client.render(input(2));
		client.render(input(3));
		expect(worker.renders()).toEqual([1]);
		worker.say(frame(1));
		expect(worker.renders()).toEqual([1, 3]);
		worker.say(frame(3));
		expect(frames).toEqual([1, 3]);
		expect(worker.renders()).toEqual([1, 3]);
	});

	it("closes a frame that is not the one in flight", () => {
		const { worker, client, frames, closed } = setup();
		client.render(input(1));
		worker.say(frame(9));
		expect(frames).toEqual([]);
		expect(closed).toHaveLength(1);
	});

	it("reports a failure, frees the slot and goes on with the waiting render", () => {
		const { worker, client, failures } = setup();
		client.render(input(1));
		client.render(input(2));
		worker.say({
			type: "error",
			seq: 1,
			reason: "missing_texture",
			detail: "itm_a",
			open: 0,
		});
		expect(failures).toEqual([
			{ seq: 1, reason: "missing_texture", detail: "itm_a" },
		]);
		expect(worker.renders()).toEqual([1, 2]);
	});

	it("resolves init with the path and rejects it with the failure", async () => {
		const a = setup();
		const ready = a.client.init("/m.wasm", "ab", "cpu");
		a.worker.say({ type: "ready", apiVersion: 3, path: "cpu", open: 0 });
		await expect(ready).resolves.toBe("cpu");
		expect(a.client.chosenPath).toBe("cpu");

		const b = setup();
		const refused = b.client.init("/m.wasm", "ab", "cpu");
		b.worker.say({
			type: "error",
			reason: "module_digest",
			detail: "x",
			open: 0,
		});
		await expect(refused).rejects.toMatchObject({ reason: "module_digest" });
	});

	it("tracks the sources the worker holds and ends the worker on dispose", () => {
		const { worker, client } = setup();
		worker.say({
			type: "error",
			reason: "invalid_argument",
			detail: "",
			open: 5,
		});
		worker.say({
			type: "error",
			reason: "invalid_argument",
			detail: "",
			open: 2,
		});
		expect(client.stats).toEqual({ workerOpen: 2, workerPeak: 5 });
		client.dispose();
		expect(worker.terminated).toBe(true);
		expect(worker.sent.at(-1)).toEqual({ type: "dispose" });
	});
});
