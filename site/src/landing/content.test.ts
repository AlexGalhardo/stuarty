// Checks the landing copy against the status column of docs/plans/00-roadmap.md (epics 88 and 95), and that the page
// holds no testimonial, customer logo, invented number or award.
import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { shots } from "../../lib/landing-pages";
import { agents, areas, hero, path, privacy, roadmap, roadmapOnlyTerms, screens, textProject, transparency } from "./content";
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
	...hero.claims.map((c) => ({ where: "hero", ...c })),
	...path.steps.flatMap((s) => s.claims.map((c) => ({ where: `path/${s.id}`, ...c }))),
	...transparency.points.map((p) => ({ where: `transparency/${p.title}`, text: p.text, epics: p.epics })),
	...textProject.claims.map((c) => ({ where: "text-project", ...c })),
	...areas.items.map((a) => ({ where: `areas/${a.id}`, text: a.text, epics: a.epics })),
	{ where: "agents", text: agents.lead, epics: agents.epics },
	...privacy.points.map((p) => ({ where: `privacy/${p.title}`, text: p.text, epics: p.epics })),
];

// Every sentence of the page: the string leaves of each export of content.ts, and the alt texts of the captures. Keys
// that are not copy (ids, paths, states) are left out.
const notCopy = new Set(["id", "src", "state", "kind", "icon", "screen"]);
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
// The alt texts of the captures that the alternative pages of the landing show (lib/landing-pages.ts) are copy too.
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
// A capture is a dark picture (name) and a light one (name-light), as .webp and .avif at 1600 and 960 px.
const captureFiles = (src: string): string[] => {
	const name = src.replace("/landing/", "");
	return ["", "-light"].flatMap((theme) => ["", "-960"].flatMap((size) => ["webp", "avif"].map((ext) => resolve(publicLanding, `${name}${theme}${size}.${ext}`))));
};

describe("the product images", () => {
	test("each has a descriptive alt text and the size of its capture", () => {
		for (const [key, s] of Object.entries(screens)) {
			expect(s.alt.length, `${key} has a short alt text`).toBeGreaterThan(40);
			expect(s.width).toBeGreaterThan(0);
			expect(s.height).toBeGreaterThan(0);
		}
	});
	test("every image exists in both themes as webp and avif at 1600 and 960 px", () => {
		for (const s of Object.values(screens)) for (const file of captureFiles(s.src)) expect(existsSync(file), `${file} is missing`).toBe(true);
	});
	test("every image is traced to a capture in CREDITS.txt, with the date of the capture", () => {
		const credits = readFileSync(resolve(publicLanding, "CREDITS.txt"), "utf8");
		for (const s of Object.values(screens)) {
			const name = s.src.replace("/landing/", "");
			expect(credits, `${name} is not in CREDITS.txt`).toMatch(new RegExp(`^${name}\\s.*\\d{4}-\\d{2}-\\d{2}$`, "m"));
		}
	});
	test("the page says when the screenshots were captured and no longer says they are old", () => {
		expect(content.capturedNote).toContain("24 November 2026");
		expect(content.capturedNote.toLowerCase()).not.toContain("has changed since");
	});
});

describe("the changes of the author after the review", () => {
	test("the page no longer says that Stuarty is in alpha", () => {
		for (const { where, text } of contentCopy) expect(text.toLowerCase(), `${where} speaks of the alpha`).not.toMatch(/alpha|rough edges/);
		const page = readFileSync(resolve(import.meta.dir, "../pages/index.astro"), "utf8");
		expect(page).not.toMatch(/rough edges/i);
	});
	test("screen recording is claimed only on Windows and the browser, with the epics that are done", () => {
		const recording = areas.items.find((a) => a.id === "recording");
		expect(recording?.epics).toEqual([62, 63, 64, 65]);
		expect(recording?.text.toLowerCase()).not.toMatch(/macos|linux/);
	});
});

describe("the design", () => {
	test("the hero and the sections keep the words of the final design", () => {
		// The author changed the hero after the review of epic 95 (2026-11-24); the change overrides the frames.
		expect(hero.title).toBe("Create, edit, manage and publish your audiovisual and text content in one place");
		expect(hero.title.startsWith(hero.titleFocus)).toBe(true);
		expect(hero.note).toBe("Free and open source. No subscriptions.");
		expect(path.steps.map((s) => s.title)).toEqual([
			"Open the home", "Start a project", "Answer the brief", "Write the script", "Bring in your media", "Cut and caption",
			"Make the thumbnail", "Work with your agent", "Teach it how you work", "Approve", "Publish", "Connect only what you choose",
		]);
		expect(new Set(path.steps.map((s) => s.screen)).size).toBe(path.steps.length);
		expect(privacy.title).toBe("On your machine. On your terms.");
		expect(privacy.subtitle).toBe("No subscriptions. No pay once. Open Source.");
		expect(areas.items).toHaveLength(12);
		expect(roadmap.items).toHaveLength(4);
		expect(roadmap.title).toBe("More of the studio, in the same place.");
	});
});
