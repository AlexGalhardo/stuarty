import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { dockerRunCommand } from "../lib/latest-release.ts";

function collectDocFiles(dir: string): string[] {
	const files: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) {
			files.push(...collectDocFiles(full));
		} else if (entry.name.endsWith(".md") || entry.name.endsWith(".mdx")) {
			files.push(full);
		}
	}
	return files;
}

test("the Docker command runs the latest image, publishes on 127.0.0.1 and mounts the data volume", () => {
	const cmd = dockerRunCommand();
	assert.ok(cmd.endsWith("ghcr.io/alexgalhardo/stuarty:latest"));
	assert.ok(cmd.includes("-p 127.0.0.1:7878:7878"));
	assert.ok(cmd.includes("-v stuarty:/data"));
	assert.ok(cmd.includes('-e APP_KEY="$(openssl rand -base64 32)"'));
	assert.ok(cmd.includes("-e STUARTY_ADMIN_EMAIL=admin@stuarty.com"));
	assert.ok(cmd.includes('-e STUARTY_ADMIN_PASSWORD="admin@stuarty.com"'));
	assert.equal(cmd.split("\n").length, 5);
	assert.ok(cmd.split("\n").slice(0, -1).every((l) => l.endsWith(" \\")));
});

test("no documentation page under site/content/docs uses the unrunnable :X.Y.Z image placeholder", () => {
	const docsRoot = join(import.meta.dirname, "..", "content", "docs");
	const offenders = collectDocFiles(docsRoot).filter((file) =>
		readFileSync(file, "utf8").includes("ghcr.io/alexgalhardo/stuarty:X.Y.Z"),
	);
	assert.deepEqual(offenders, []);
});
