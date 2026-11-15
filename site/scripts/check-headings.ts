// Every Portuguese page must have the same headings, by level and in the same order, as its English page
// (SDD-010 section 6). Headings inside fenced code blocks and the front matter are ignored.
// Run from site/: `npm run check:headings`. Node strips the types (no build step).
import fs from "node:fs";
import path from "node:path";

export type HeadingMismatch = { page: string; english: number[]; portuguese: number[] };

/** The levels of the ATX headings of an MDX source, in order. */
export function headingLevels(source: string): number[] {
	const levels: number[] = [];
	let fence: string | undefined;
	let lineNo = 0;
	let frontMatter = false;
	for (const line of source.replace(/\r\n/g, "\n").split("\n")) {
		lineNo++;
		if (lineNo === 1 && line.trim() === "---") {
			frontMatter = true;
			continue;
		}
		if (frontMatter) {
			if (line.trim() === "---") frontMatter = false;
			continue;
		}
		const f = /^\s{0,3}(`{3,}|~{3,})/.exec(line);
		if (f) {
			const marker = f[1] as string;
			if (fence === undefined) fence = marker;
			else if (marker[0] === fence[0] && marker.length >= fence.length) fence = undefined;
			continue;
		}
		if (fence !== undefined) continue;
		const h = /^ {0,3}(#{1,6})\s+\S/.exec(line);
		if (h) levels.push((h[1] as string).length);
	}
	return levels;
}

/** The pages of `contentDir/docs/en` whose Portuguese twin has other headings. A missing twin is not listed here. */
export function checkHeadings(contentDir: string): HeadingMismatch[] {
	const enRoot = path.join(contentDir, "docs", "en");
	const walk = (dir: string): string[] =>
		fs
			.readdirSync(dir, { withFileTypes: true })
			.flatMap((e) => (e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]));
	const out: HeadingMismatch[] = [];
	for (const abs of fs.existsSync(enRoot) ? walk(enRoot) : []) {
		if (!abs.endsWith(".mdx")) continue;
		const page = path.relative(enRoot, abs).split(path.sep).join("/").replace(/\.mdx$/, "");
		const pt = path.join(contentDir, "docs", "pt-br", `${page}.mdx`);
		if (!fs.existsSync(pt)) continue;
		const english = headingLevels(fs.readFileSync(abs, "utf8"));
		const portuguese = headingLevels(fs.readFileSync(pt, "utf8"));
		if (english.length !== portuguese.length || english.some((l, i) => l !== portuguese[i])) {
			out.push({ page, english, portuguese });
		}
	}
	return out.sort((a, b) => a.page.localeCompare(b.page));
}

/** Prints the mismatches and returns the exit code. */
export function reportHeadings(mismatches: HeadingMismatch[]): number {
	for (const m of mismatches) {
		console.log(`::error file=site/content/docs/pt-br/${m.page}.mdx::headings differ: en [${m.english.join(",")}] pt-BR [${m.portuguese.join(",")}]`);
	}
	console.log(`check:headings: ${mismatches.length} page(s) with different headings`);
	return mismatches.length === 0 ? 0 : 1;
}

if (import.meta.main) {
	process.exit(reportHeadings(checkHeadings(path.join(import.meta.dirname, "..", "content"))));
}
