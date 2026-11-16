import assert from "node:assert/strict";
import { test } from "node:test";
import { dockerRunCommand, latestVersion } from "../lib/latest-release.ts";

const changelog = "# Changelog\n\n## [Unreleased]\n\n## [0.17.0] - 2026-11-16\n\n### Added\n\n## [0.16.0] - 2026-11-16\n";

test("the latest version is the first released heading, not [Unreleased]", () => {
	assert.equal(latestVersion(changelog), "0.17.0");
});

test("a changelog with no release is an error, not a guess", () => {
	assert.throws(() => latestVersion("# Changelog\n\n## [Unreleased]\n"), /no released version/);
});

test("the Docker command pins the version, publishes on 127.0.0.1 and mounts the data volume", () => {
	const cmd = dockerRunCommand("0.17.0");
	assert.ok(cmd.endsWith("ghcr.io/alexgalhardo/vukem-video:0.17.0"));
	assert.ok(cmd.includes("-p 127.0.0.1:7878:7878"));
	assert.ok(cmd.includes("-v vukem-video:/data"));
	assert.ok(cmd.includes('-e APP_KEY="$(openssl rand -base64 32)"'));
	assert.ok(!cmd.includes(":latest"));
	assert.equal(cmd.split("\n").length, 5);
	assert.ok(cmd.split("\n").slice(0, -1).every((l) => l.endsWith(" \\")));
});
