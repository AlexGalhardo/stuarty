import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import { isCompletePng, makeOnce } from "./makeOnce";

// The smallest valid PNG: one transparent pixel.
const PNG = Buffer.from(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
	"base64",
);

const dirs: string[] = [];
afterEach(() => {
	for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

function scratch(): string {
	const d = mkdtempSync(join(tmpdir(), "make-once-"));
	dirs.push(d);
	return d;
}

describe("makeOnce with a PNG", () => {
	it("recognises a whole PNG and refuses a truncated one", () => {
		const dir = scratch();
		const whole = join(dir, "whole.png");
		const cut = join(dir, "cut.png");
		writeFileSync(whole, PNG);
		writeFileSync(cut, PNG.subarray(0, PNG.length - 7));
		expect(isCompletePng(whole)).toBe(true);
		expect(isCompletePng(cut)).toBe(false);
		expect(isCompletePng(join(dir, "missing.png"))).toBe(false);
	});

	it("rebuilds a truncated file and leaves a whole one alone", () => {
		const out = join(scratch(), "photo-0.png");
		writeFileSync(out, PNG.subarray(0, 20));
		let made = 0;
		const make = (target: string): void => {
			made++;
			writeFileSync(target, PNG);
		};
		makeOnce(out, make, isCompletePng);
		expect(made).toBe(1);
		expect(readFileSync(out).equals(PNG)).toBe(true);
		makeOnce(out, make, isCompletePng);
		expect(made).toBe(1);
	});
});
