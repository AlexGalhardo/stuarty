import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { checkTranslations, exitCode, statusOf, type Row } from "./check-translations.ts";

/** A scratch repository with a content/docs tree; each commit has a fixed author date. */
function repo(): { dir: string; commit: (date: string, message: string, files: Record<string, string>) => void } {
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), "translations-"));
	const git = (...args: string[]) => execFileSync("git", args, { cwd: dir, env: { ...process.env, ...env("2026-01-01T00:00:00Z") } });
	function env(date: string) {
		return {
			GIT_AUTHOR_DATE: date,
			GIT_COMMITTER_DATE: date,
			GIT_AUTHOR_NAME: "t",
			GIT_AUTHOR_EMAIL: "t@example.org",
			GIT_COMMITTER_NAME: "t",
			GIT_COMMITTER_EMAIL: "t@example.org",
		};
	}
	git("init", "-q");
	return {
		dir,
		commit(date, message, files) {
			for (const [rel, text] of Object.entries(files)) {
				fs.mkdirSync(path.dirname(path.join(dir, rel)), { recursive: true });
				fs.writeFileSync(path.join(dir, rel), text);
			}
			execFileSync("git", ["add", "."], { cwd: dir });
			execFileSync("git", ["commit", "-q", "-m", message], { cwd: dir, env: { ...process.env, ...env(date) } });
		},
	};
}

const en = (page: string) => `docs/en/${page}.mdx`;
const pt = (page: string) => `docs/pt-br/${page}.mdx`;
const statusOfPage = (rows: Row[], page: string) => rows.find((r) => r.page === page)?.status;

test("both files changed in one commit are ok", () => {
	const r = repo();
	r.commit("2026-02-01T00:00:00Z", "add", { [en("a")]: "1", [pt("a")]: "1" });
	assert.equal(statusOfPage(checkTranslations(r.dir), "a"), "ok");
});

test("English changed after the translation is stale", () => {
	const r = repo();
	r.commit("2026-02-01T00:00:00Z", "add", { [en("a")]: "1", [pt("a")]: "1" });
	r.commit("2026-03-01T00:00:00Z", "change", { [en("a")]: "2" });
	assert.equal(statusOfPage(checkTranslations(r.dir), "a"), "stale");
});

test("a newer English commit with Translation: not-needed is ok", () => {
	const r = repo();
	r.commit("2026-02-01T00:00:00Z", "add", { [en("a")]: "1", [pt("a")]: "1" });
	r.commit("2026-03-01T00:00:00Z", "fix a typo\n\nTranslation: not-needed", { [en("a")]: "2" });
	assert.equal(statusOfPage(checkTranslations(r.dir), "a"), "ok");
});

test("a missing translation is reported", () => {
	const r = repo();
	r.commit("2026-02-01T00:00:00Z", "add", { [en("a")]: "1" });
	assert.equal(statusOfPage(checkTranslations(r.dir), "a"), "missing");
});

test("the translation updated later than English is ok", () => {
	const r = repo();
	r.commit("2026-02-01T00:00:00Z", "add", { [en("a")]: "1", [pt("a")]: "1" });
	r.commit("2026-03-01T00:00:00Z", "change", { [en("a")]: "2" });
	r.commit("2026-03-02T00:00:00Z", "translate", { [pt("a")]: "2" });
	assert.equal(statusOfPage(checkTranslations(r.dir), "a"), "ok");
});

test("only a strict run fails, and only for a stale or missing page", () => {
	const ok: Row[] = [{ page: "a", status: "ok" }];
	const stale: Row[] = [{ page: "a", status: "stale" }];
	assert.equal(exitCode(stale, false), 0);
	assert.equal(exitCode(stale, true), 1);
	assert.equal(exitCode([{ page: "b", status: "missing" }], true), 1);
	assert.equal(exitCode(ok, true), 0);
});

test("a file with no history counts as changed at HEAD", () => {
	const head = "2026-05-01T00:00:00Z";
	assert.equal(statusOf([], [{ hash: "p", date: "2026-02-01T00:00:00Z", message: "" }], head), "stale");
	assert.equal(statusOf([], undefined, head), "missing");
});
