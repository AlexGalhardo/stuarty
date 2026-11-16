import fs from "node:fs";
import path from "node:path";

/** The repository CHANGELOG.md is the single source of the release list (the site reads it at build time, no network). */
const CHANGELOG = path.join(process.cwd(), "..", "CHANGELOG.md");

/** The newest released version: the first `## [X.Y.Z]` heading (`[Unreleased]` does not count). */
export function latestVersion(changelog: string = fs.readFileSync(CHANGELOG, "utf8")): string {
	const match = /^## \[(\d+\.\d+\.\d+)\]/m.exec(changelog);
	if (!match) throw new Error("CHANGELOG.md has no released version heading (## [X.Y.Z])");
	return match[1];
}

export const IMAGE = "ghcr.io/alexgalhardo/vukem-video";

/**
 * The one command that starts Vukem Video from the container image of a release. The flags are those of the
 * Docker page: the port is published on this computer only, the workspace is the /data volume, the container
 * runs in network mode (APP_KEY and a first administrator are required).
 */
export function dockerRunCommand(version: string): string {
	return [
		"docker run --rm -p 127.0.0.1:7878:7878 -v vukem-video:/data",
		'-e APP_KEY="$(openssl rand -base64 32)"',
		"-e VUKEM_VIDEO_ADMIN_EMAIL=admin@example.com",
		"-e VUKEM_VIDEO_ADMIN_PASSWORD=choose-a-long-password",
		`${IMAGE}:${version}`,
	].join(" \\\n  ");
}
