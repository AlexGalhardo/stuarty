// Checks the landing copy against the status column of docs/plans/00-roadmap.md (epic 88, task 4), and that the page
// holds no testimonial, customer logo, invented number or award.
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { agents, areas, path, privacy, roadmap, roadmapOnlyTerms, screenSlots, textProject, transparency } from "./content";
import * as content from "./content";

const roadmapFile = resolve(import.meta.dir, "../../../docs/plans/00-roadmap.md");

function statuses(): Map<number, string> {
	const out = new Map<number, string>();
	for (const line of readFileSync(roadmapFile, "utf8").split(/\r?\n/)) {
		const m = /^\| \[(\d+)\]\([^)]*\) \|.*\| ([^|]+) \|$/.exec(line);
		if (m) out.set(Number(m[1]), m[2].trim());
	}
	return out;
}

const status = statuses();
const done = (n: number): boolean => (status.get(n) ?? "").startsWith("DONE");

// Every claim of the released sections, with the epics that deliver it.
const released: { where: string; text: string; epics: number[] }[] = [
	...path.steps.flatMap((s) => s.claims.map((c) => ({ where: `path/${s.id}`, ...c }))),
	...transparency.points.map((p) => ({ where: `transparency/${p.title}`, text: p.text, epics: p.epics })),
	...textProject.claims.map((c) => ({ where: "text-project", ...c })),
	...areas.items.map((a) => ({ where: `areas/${a.id}`, text: a.text, epics: a.epics })),
	{ where: "agents", text: agents.lead, epics: agents.epics },
	...privacy.points.map((p) => ({ where: `privacy/${p.title}`, text: p.text, epics: p.epics })),
];

describe("the roadmap file", () => {
	test("is parsed", () => {
		expect(status.size).toBeGreaterThan(60);
		expect(status.get(88)).toBeDefined();
	});
});

describe("released claims", () => {
	for (const c of released) {
		test(`${c.where}: every epic is DONE`, () => {
			for (const n of c.epics) {
				expect(status.has(n), `epic ${n} is not in the roadmap`).toBe(true);
				expect(done(n), `epic ${n} is ${status.get(n)}`).toBe(true);
			}
		});
	}
});

describe("On the roadmap", () => {
	for (const item of roadmap.items) {
		test(`${item.id} is stated as ${item.state}`, () => {
			const s = item.epics.map((n) => {
				expect(status.has(n), `epic ${n} is not in the roadmap`).toBe(true);
				return status.get(n) ?? "";
			});
			if (item.state === "in-progress") {
				expect(s.some((x) => x.startsWith("IN PROGRESS"))).toBe(true);
				expect(s.every((x) => !x.startsWith("DONE"))).toBe(true);
			} else if (item.state === "planned") {
				expect(s.every((x) => !x.startsWith("DONE"))).toBe(true);
			} else {
				expect(s.every((x) => x.startsWith("DONE"))).toBe(true);
			}
		});
	}

	test("a feature that is not released appears only in the roadmap", () => {
		for (const c of released) {
			const lower = c.text.toLowerCase();
			for (const term of roadmapOnlyTerms) expect(lower, `${c.where} says "${term}"`).not.toContain(term);
		}
		const outside = [content.hero.title, content.hero.lead, ...content.hero.facts, content.path.lead, content.downloads.lead].join(" ").toLowerCase();
		for (const term of roadmapOnlyTerms) expect(outside).not.toContain(term);
	});
});

describe("what the page never holds", () => {
	const all = JSON.stringify(content).toLowerCase();
	test("no testimonial, customer logo or award", () => {
		for (const word of ["testimonial", "trusted by", "customers", "award", "loved by", "5 stars", "rated"]) expect(all).not.toContain(word);
	});
	test("no invented number", () => {
		expect(all).not.toMatch(/\d[\d,.]*\s?(k|m|\+|%)?\s+(users|creators|customers|downloads|stars|teams|videos|projects)/);
		expect(all).not.toMatch(/\d+x\s+faster|\d+%/);
	});
});

describe("the slots of epic 79", () => {
	test("each has a label and a descriptive alt text", () => {
		expect(Object.keys(screenSlots).sort()).toEqual(["briefing", "home", "projects"]);
		for (const s of Object.values(screenSlots)) {
			expect(s.label.length).toBeGreaterThan(8);
			expect(s.alt.length).toBeGreaterThan(30);
		}
	});
});
