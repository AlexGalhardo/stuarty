import { describe, expect, test } from "vitest";

import { createImportQueue, parsePastedPaths } from "./importPaths";

describe("parsePastedPaths", () => {
	test("one path per line, quotes stripped", () => {
		expect(parsePastedPaths('"C:a b.mp4"\r\n/c.wav\n\n')).toEqual([
			"C:a b.mp4",
			"/c.wav",
		]);
	});
});

describe("createImportQueue", () => {
	test("paths added during an import wait and run in order", async () => {
		const ran: string[][] = [];
		const busy: boolean[] = [];
		let release: () => void = () => {};
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		const queue = createImportQueue(
			async (paths) => {
				ran.push([...paths]);
				if (ran.length === 1) await gate;
			},
			(b) => busy.push(b),
		);

		queue.add(["/a.mp4"]);
		expect(queue.busy).toBe(true);
		queue.add(["/b.mp4", "/c.mp4"]);
		expect(ran).toEqual([["/a.mp4"]]);

		release();
		await new Promise((resolve) => setTimeout(resolve, 0));
		expect(ran).toEqual([["/a.mp4"], ["/b.mp4", "/c.mp4"]]);
		expect(queue.busy).toBe(false);
		expect(busy).toEqual([true, false]);
	});
});
