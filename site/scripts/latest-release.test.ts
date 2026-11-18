import assert from "node:assert/strict";
import { test } from "node:test";
import { dockerRunCommand } from "../lib/latest-release.ts";

test("the Docker command runs the latest image, publishes on 127.0.0.1 and mounts the data volume", () => {
	const cmd = dockerRunCommand();
	assert.ok(cmd.endsWith("ghcr.io/alexgalhardo/vukem-video:latest"));
	assert.ok(cmd.includes("-p 127.0.0.1:7878:7878"));
	assert.ok(cmd.includes("-v vukem-video:/data"));
	assert.ok(cmd.includes('-e APP_KEY="$(openssl rand -base64 32)"'));
	assert.equal(cmd.split("\n").length, 5);
	assert.ok(cmd.split("\n").slice(0, -1).every((l) => l.endsWith(" \\")));
});
