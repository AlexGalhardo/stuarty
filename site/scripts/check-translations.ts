// Lists the pages whose English text changed after their Portuguese twin (SDD-010 section 6): dates come
// from git (author date, so a rebase merge keeps them) and full history is required, like check:last-updated.
// A commit whose message has a line "Translation: not-needed" does not make a translation stale.
// Run from site/: `npm run check:translations [-- --strict]`. Node strips the types (no build step).
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

export type Status = "ok" | "stale" | "missing";

export type Row = { page: string; english?: string; portuguese?: string; status: Status };

type Commit = { hash: string; date: string; message: string };

/** The commits that touched a file, newest first. */
function commitsOf(contentDir: string, file: string): Commit[] {
	const out = execFileSync("git", ["log", "--format=%x00%H%n%aI%n%B", "--", file], {
		cwd: contentDir,
		encoding: "utf8",
		maxBuffer: 1 << 26,
	});
	return out
		.split("\0")
		.filter((block) => block.trim() !== "")
		.map((block) => {
			const [hash = "", date = "", ...message] = block.split("\n");
			return { hash: hash.trim(), date: date.trim(), message: message.join("\n") };
		});
}

const NOT_NEEDED = /^Translation:\s*not-needed\s*$/im;

/** The status of one pair, from the commits of its two files. Pure, so it is tested without git. */
export function statusOf(english: Commit[], portuguese: Commit[] | undefined, headDate: string): Row["status"] {
	if (portuguese === undefined) return "missing";
	// A file with no history yet counts as changed at the checkout HEAD (a page in a pull request).
	const tEn = english.find((c) => !NOT_NEEDED.test(c.message))?.date ?? (english.length === 0 ? headDate : undefined);
	const tPt = portuguese[0]?.date ?? headDate;
	if (english[0] && portuguese[0] && english[0].hash === portuguese[0].hash) return "ok";
	if (tEn === undefined) return "ok";
	return Date.parse(tEn) > Date.parse(tPt) ? "stale" : "ok";
}

/** Every English page of `contentDir/docs/en` against its Portuguese twin. */
export function checkTranslations(contentDir: string): Row[] {
	const head = execFileSync("git", ["log", "-1", "--format=%aI", "HEAD"], { cwd: contentDir, encoding: "utf8" }).trim();
	const enRoot = path.join(contentDir, "docs", "en");
	const walk = (dir: string): string[] =>
		fs
			.readdirSync(dir, { withFileTypes: true })
			.flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
	const rows: Row[] = [];

	for (const abs of fs.existsSync(enRoot) ? walk(enRoot) : []) {
		if (!abs.endsWith(".mdx")) continue;
		const page = path.relative(enRoot, abs).split(path.sep).join("/").replace(/\.mdx$/, "");
		const pt = path.join(contentDir, "docs", "pt-br", `${page}.mdx`);
		const english = commitsOf(contentDir, abs);
		const portuguese = fs.existsSync(pt) ? commitsOf(contentDir, pt) : undefined;
		rows.push({
			page,
			english: english[0]?.date ?? head,
			portuguese: portuguese === undefined ? undefined : (portuguese[0]?.date ?? head),
			status: statusOf(english, portuguese, head),
		});
	}

	const order: Record<Status, number> = { stale: 0, missing: 1, ok: 2 };
	return rows.sort((a, b) => order[a.status] - order[b.status] || a.page.localeCompare(b.page));
}

export function toTable(rows: Row[]): string {
	return [
		"| page | English date | pt-BR date | status |",
		"| --- | --- | --- | --- |",
		...rows.map((r) => `| ${r.page} | ${r.english ?? ""} | ${r.portuguese ?? ""} | ${r.status} |`),
	].join("\n");
}

/** The exit code: 0 unless the check is strict and a page is stale or missing. */
export function exitCode(rows: Row[], strict: boolean): number {
	return strict && rows.some((r) => r.status !== "ok") ? 1 : 0;
}

function main(): number {
	const contentDir = path.join(import.meta.dirname, "..", "content");
	const config = JSON.parse(fs.readFileSync(path.join(contentDir, "translations.config.json"), "utf8")) as {
		strict: boolean;
	};
	const strict = config.strict || process.argv.includes("--strict");
	const rows = checkTranslations(contentDir);
	const table = toTable(rows);

	console.log(table);
	if (process.env.GITHUB_STEP_SUMMARY) fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${table}\n`);
	for (const r of rows) {
		if (r.status !== "ok") console.log(`::warning file=site/content/docs/pt-br/${r.page}.mdx::${r.status}`);
	}

	const bad = rows.filter((r) => r.status !== "ok").length;
	console.log(`check:translations: ${rows.length} pages, ${bad} stale or missing${strict ? " (strict)" : ""}`);
	return exitCode(rows, strict);
}

if (import.meta.main) process.exit(main());
