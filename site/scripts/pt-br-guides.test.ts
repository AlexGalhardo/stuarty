import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";

const root = path.join(import.meta.dirname, "..", "..");
const guides = ["comecar.md", "primeiro-short.md", "recipe-product-commercial.md", "recipe-long-talking-video.md"];

// The site is English only (SDD-010 section 15.1); the Portuguese guides live in docs/guides/pt-BR/ and the
// introduction links to them. A guide that is deleted or unlinked would leave the introduction with a dead link.
test("the Portuguese guides exist and the introduction links to each", () => {
	const intro = fs.readFileSync(path.join(root, "site", "content", "docs", "en", "getting-started", "introduction.mdx"), "utf8");
	for (const g of guides) {
		const text = fs.readFileSync(path.join(root, "docs", "guides", "pt-BR", g), "utf8");
		assert.match(text, /^# \S/, `${g} has no title`);
		assert.ok(intro.includes(`/docs/guides/pt-BR/${g}`), `the introduction does not link ${g}`);
	}
});

test("the commands a guide names are commands of the CLI", () => {
	const cli = JSON.parse(fs.readFileSync(path.join(root, "site", "generated", "cli.json"), "utf8")) as { commands: { name: string }[] };
	const names = new Set(cli.commands.map((c) => c.name));
	for (const g of guides) {
		const text = fs.readFileSync(path.join(root, "docs", "guides", "pt-BR", g), "utf8");
		for (const [, cmd] of text.matchAll(/stuarty ([a-z][a-z:-]*)/g)) {
			assert.ok(names.has(cmd as string), `${g}: stuarty ${cmd} is not a command`);
		}
	}
});
