import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const dir = fileURLToPath(new URL("../e2e/reference-machine/", import.meta.url));

interface Check {
	name: string;
	ok: boolean;
	detail: string;
}

interface Record {
	recorded_on: string;
	machine: { cpu: string; gpu: string; os: string; logical_cpus: number };
	command: string;
	doctor: { version: string; checks: Check[] };
}

const files = readdirSync(dir).filter((f) => /^doctor-\d{4}-\d{2}-\d{2}\.json$/.test(f));

describe("reference machine records", () => {
	it("has at least one doctor record", () => {
		expect(files.length).toBeGreaterThan(0);
	});

	for (const file of files) {
		it(`${file} names the hardware and the checks a later run compares`, () => {
			const rec = JSON.parse(readFileSync(dir + file, "utf8")) as Record;
			expect(file).toBe(`doctor-${rec.recorded_on}.json`);
			expect(rec.command).toBe("stuarty doctor --full --json");
			expect(rec.machine.cpu).not.toBe("");
			expect(rec.machine.logical_cpus).toBeGreaterThan(0);
			const names = rec.doctor.checks.map((c) => c.name);
			for (const need of ["system", "ffmpeg.binary", "ffmpeg.encoder", "ffmpeg.license"]) {
				expect(names).toContain(need);
			}
			const license = rec.doctor.checks.find((c) => c.name === "ffmpeg.license");
			expect(license?.detail).toMatch(/^lgpl/);
		});
	}
});
