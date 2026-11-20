import { describe, expect, it } from "vitest";

import { createSlotQueue, type Release, whileNear } from "./slotQueue";

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

describe("whileNear", () => {
	it("withdraws a card that left the viewport before its turn, so the cards on screen go first (VV-238)", () => {
		const queue = createSlotQueue(1);
		const started: string[] = [];
		const releases: Release[] = [];
		const ticket = (name: string) =>
			whileNear(queue, (release) => {
				started.push(name);
				releases.push(release);
			});

		const first = ticket("first");
		const passed = ticket("scrolled past");
		const onScreen = ticket("on screen");

		first.near(true);
		passed.near(true);
		passed.near(false);
		onScreen.near(true);
		releases.shift()?.();

		expect(started).toEqual(["first", "on screen"]);
	});

	it("keeps the slot of a request already in flight when the card leaves, and frees it on stop", () => {
		const queue = createSlotQueue(1);
		const started: string[] = [];

		const inFlight = whileNear(queue, () => started.push("a"));
		inFlight.near(true);
		inFlight.near(false);
		inFlight.near(true);

		const next = whileNear(queue, () => started.push("b"));
		next.near(true);
		expect(started).toEqual(["a"]);

		inFlight.stop();
		expect(started).toEqual(["a", "b"]);
	});

	it("asks again when a withdrawn card comes back", () => {
		const queue = createSlotQueue(2);
		const started: string[] = [];
		const ticket = whileNear(queue, () => started.push("a"));

		ticket.near(false);
		expect(started).toEqual([]);
		ticket.near(true);
		expect(started).toEqual(["a"]);
	});
});
