import { describe, expect, it } from "vitest";

import { createSlotQueue, type Release } from "./slotQueue";

describe("createSlotQueue", () => {
	it("never runs more than the limit at once (VV-238)", () => {
		const queue = createSlotQueue(2);
		const releases: Release[] = [];
		let running = 0;
		let peak = 0;

		for (let i = 0; i < 6; i++) {
			queue.acquire((release) => {
				running++;
				peak = Math.max(peak, running);
				releases.push(() => {
					running--;
					release();
				});
			});
		}

		expect(running).toBe(2);

		while (releases.length > 0) releases.shift()?.();

		expect(peak).toBe(2);
		expect(running).toBe(0);
	});

	it("starts the oldest waiter when a slot is freed", () => {
		const queue = createSlotQueue(1);
		const order: string[] = [];
		let first: Release = () => {};

		queue.acquire((release) => {
			order.push("a");
			first = release;
		});
		queue.acquire((release) => {
			order.push("b");
			release();
		});
		queue.acquire((release) => {
			order.push("c");
			release();
		});

		expect(order).toEqual(["a"]);
		first();
		expect(order).toEqual(["a", "b", "c"]);
	});

	it("drops a withdrawn waiter and frees the slot of a withdrawn runner", () => {
		const queue = createSlotQueue(1);
		const started: string[] = [];

		const withdrawA = queue.acquire(() => started.push("a"));
		const withdrawB = queue.acquire(() => started.push("b"));
		queue.acquire((release) => {
			started.push("c");
			release();
		});

		withdrawB();
		withdrawA();
		withdrawA();

		expect(started).toEqual(["a", "c"]);
	});
});
