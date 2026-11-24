// Checks the landing copy against the status column of docs/plans/00-roadmap.md (epic 88, task 4), and that the page
// holds no testimonial, customer logo, invented number or award.
import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { shots } from "../../lib/landing-pages";
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

// Every sentence of the page: the string leaves of each export of content.ts, and the alt texts of the captures. Keys
// that are not copy (ids, paths, states) are left out.
const notCopy = new Set(["id", "src", "srcDark", "state", "kind", "shot", "slot"]);
function strings(value: unknown, where: string, out: { where: string; text: string }[]): { where: string; text: string }[] {
	if (typeof value === "string") out.push({ where, text: value });
	else if (Array.isArray(value)) value.forEach((v, i) => strings(v, `${where}[${i}]`, out));
	else if (value && typeof value === "object") {
		for (const [k, v] of Object.entries(value)) if (!notCopy.has(k)) strings(v, `${where}.${k}`, out);
	}
	return out;
}
const pageCopy = Object.entries(content)
	.filter(([k]) => k !== "roadmap" && k !== "roadmapOnlyTerms")
	.flatMap(([k, v]) => strings(v, k, []));
// The alt texts describe the captures, so they may give a real size such as "1280 by 720"; they are checked for words.
const shotAlts = Object.entries(shots).map(([k, s]) => ({ where: `shots.${k}.alt`, text: s.alt }));
const copyOutsideRoadmap = [...pageCopy, ...shotAlts];
const contentCopy = [...pageCopy, ...strings(roadmap, "roadmap", [])];
const allCopy = [...contentCopy, ...shotAlts];

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
	// An epic DONE on the integration branch ships with this page, so it is never on the roadmap, and the label of an
	// item is exact: "In progress" only when one of its epics is IN PROGRESS, "Planned" only when none is.
	for (const item of roadmap.items) {
		test(`${item.id} is stated as ${item.state}`, () => {
			const s = item.epics.map((n) => {
				expect(status.has(n), `epic ${n} is not in the roadmap`).toBe(true);
				return status.get(n) ?? "";
			});
			for (const [i, x] of s.entries()) expect(x.startsWith("DONE"), `epic ${item.epics[i]} is DONE: move it out of the roadmap`).toBe(false);
			expect(s.some((x) => x.startsWith("IN PROGRESS"))).toBe(item.state === "in-progress");
		});
	}

	test("a feature that is not released appears only in the roadmap", () => {
		for (const { where, text } of copyOutsideRoadmap) {
			const lower = text.toLowerCase();
			for (const term of roadmapOnlyTerms) expect(lower, `${where} says "${term}"`).not.toContain(term);
		}
	});
});

describe("what the page never holds", () => {
	const lowerAll = allCopy.map((c) => ({ ...c, text: c.text.toLowerCase() }));
	test("no testimonial, customer logo, award or boast", () => {
		const banned = [
			"testimonial", "trusted by", "used by", "loved by", "customer", "award", "winner", "as seen", "featured in", "rated",
			"stars", "review from", "said", "best", "fastest", "blazing", "lightning", "world-class", "#1", "military-grade",
			"bank-level", "unhackable", "100%",
		];
		for (const { where, text } of lowerAll) {
			for (const word of banned) {
				const escaped = word.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
				expect(text, `${where} says "${word}"`).not.toMatch(new RegExp(`(^|[^a-z0-9])${escaped}($|[^a-z0-9])`));
			}
		}
	});
	test("no number, except the date of the screenshots", () => {
		for (const { where, text } of contentCopy) {
			const rest = where === "capturedNote" ? text.replace("24 November 2026", "") : text;
			// A digit inside a name such as MP4 is not a number.
			expect(rest, `${where} holds a number`).not.toMatch(/(^|[^A-Za-z])\d/);
			expect(rest.toLowerCase(), `${where} holds a number in words`).not.toMatch(/\b(dozens|hundreds|thousands|millions|billions)\b|\btimes (faster|smaller|cheaper)\b/);
		}
	});
	test("the page draws no image or link of its own, only the captures and the copy of this module", () => {
		const page = readFileSync(resolve(import.meta.dir, "../pages/index.astro"), "utf8");
		expect(page).not.toMatch(/<img\b|<svg\b|https?:\/\//);
	});
});

const publicLanding = resolve(import.meta.dir, "../../public/landing");
const captureFiles = (base: string): string[] =>
	["", "-960"].flatMap((size) => ["webp", "avif"].map((ext) => resolve(publicLanding, `${base.replace("/landing/", "")}${size}.${ext}`)));

describe("the screens of epic 79", () => {
	test("each has a label, a descriptive alt text and a capture in both themes", () => {
		expect(Object.keys(screenSlots).sort()).toEqual(["briefing", "home", "projects"]);
		for (const s of Object.values(screenSlots)) {
			expect(s.label.length).toBeGreaterThan(8);
			expect(s.alt.length).toBeGreaterThan(30);
			expect(s.src, `${s.label} has no light capture`).toBeDefined();
			expect(s.srcDark, `${s.label} has no dark capture`).toBeDefined();
		}
	});
});

describe("the product images", () => {
	const bases = [
		...Object.values(screenSlots).flatMap((s) => [s.src ?? "", s.srcDark ?? ""]),
		...Object.values(shots).flatMap((s) => [s.src, `${s.src.replace(/-light$/, "")}-light`]),
	];
	test("every image exists as webp and avif at 1600 and 960 px", () => {
		for (const base of new Set(bases)) for (const file of captureFiles(base)) expect(existsSync(file), `${file} is missing`).toBe(true);
	});
	test("every image is traced to a capture in CREDITS.txt, with the date of the capture", () => {
		const credits = readFileSync(resolve(publicLanding, "CREDITS.txt"), "utf8");
		for (const base of new Set(bases)) {
			const name = base.replace("/landing/", "").replace(/-light$/, "");
			expect(credits, `${name} is not in CREDITS.txt`).toMatch(new RegExp(`^${name}\\s.*\\d{4}-\\d{2}-\\d{2}$`, "m"));
		}
		expect(credits).not.toContain("2026-11-20");
	});
	test("the page says when the screenshots were captured and no longer says they are old", () => {
		expect(content.capturedNote).toContain("24 November 2026");
		expect(content.capturedNote.toLowerCase()).not.toContain("has changed since");
	});
});
