import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { FALLBACK_TAG, pickDownloads, type LatestRelease } from "../lib/releases.ts";

const REPO = "https://github.com/AlexGalhardo/stuarty";
const asset = (name: string) => ({ name, browser_download_url: `${REPO}/releases/download/v1.2.3/${name}` });

// Each call imports a fresh copy of the module: latestRelease caches its answer for the whole build.
let copies = 0;
async function fresh(): Promise<() => Promise<LatestRelease>> {
	copies += 1;
	const mod = (await import(`../lib/releases.ts?copy=${copies}`)) as typeof import("../lib/releases.ts");
	return mod.latestRelease;
}

const realFetch = globalThis.fetch;
const realToken = process.env.GITHUB_TOKEN;
afterEach(() => {
	globalThis.fetch = realFetch;
	if (realToken === undefined) delete process.env.GITHUB_TOKEN;
	else process.env.GITHUB_TOKEN = realToken;
});

test("the desktop installer of each system wins over its server archive, and the arm64 disk image on macOS", () => {
	const downloads = pickDownloads("v1.2.3", [
		asset("stuarty_1.2.3_windows_amd64.zip"),
		asset("stuarty-desktop-1-2-3.exe"),
		asset("stuarty_1.2.3_darwin_arm64.tar.gz"),
		asset("stuarty-desktop-1-2-3-amd64.dmg"),
		asset("stuarty-desktop-1-2-3-arm64.dmg"),
		asset("stuarty_1.2.3_linux_arm64.deb"),
		asset("stuarty_1.2.3_linux_amd64.deb"),
		asset("stuarty-desktop-1-2-3.AppImage"),
	]);
	assert.deepEqual(
		downloads.map((d) => [d.os, d.href.split("/").pop(), d.direct]),
		[
			["windows", "stuarty-desktop-1-2-3.exe", true],
			["macos", "stuarty-desktop-1-2-3-arm64.dmg", true],
			["linux", "stuarty-desktop-1-2-3.AppImage", true],
		],
	);
	assert.ok(downloads.every((d) => d.href.startsWith(`${REPO}/releases/download/v1.2.3/`)));
});

test("with only server archives, each system gets its archive, and the amd64 .deb before the arm64 one", () => {
	const downloads = pickDownloads("v1.2.3", [
		asset("stuarty_1.2.3_linux_arm64.deb"),
		asset("stuarty_1.2.3_linux_amd64.deb"),
		asset("stuarty_1.2.3_windows_amd64.zip"),
		asset("stuarty_1.2.3_darwin_arm64.tar.gz"),
	]);
	assert.deepEqual(
		downloads.map((d) => d.href.split("/").pop()),
		["stuarty_1.2.3_windows_amd64.zip", "stuarty_1.2.3_darwin_arm64.tar.gz", "stuarty_1.2.3_linux_amd64.deb"],
	);
});

test("a system with no matching asset opens the latest release page", () => {
	const downloads = pickDownloads("v1.2.3", [asset("checksums.txt")]);
	assert.deepEqual(
		downloads.map((d) => [d.os, d.label, d.href, d.direct]),
		[
			["windows", "Download for Windows", `${REPO}/releases/latest`, false],
			["macos", "Download for macOS", `${REPO}/releases/latest`, false],
			["linux", "Download for Linux", `${REPO}/releases/latest`, false],
		],
	);
});

test("a network failure falls back to the committed release and never throws", async () => {
	globalThis.fetch = async () => {
		throw new TypeError("fetch failed");
	};
	const release = await (await fresh())();
	assert.equal(release.tag, FALLBACK_TAG);
	assert.equal(release.version, FALLBACK_TAG.replace(/^v/, ""));
	assert.ok(release.downloads.every((d) => !d.direct));
});

test("an error status or a body without a tag falls back too", async () => {
	globalThis.fetch = async () => new Response("rate limited", { status: 403 });
	assert.equal((await (await fresh())()).tag, FALLBACK_TAG);
	globalThis.fetch = async () => Response.json({ message: "Not Found" });
	assert.equal((await (await fresh())()).tag, FALLBACK_TAG);
});

test("the latest release is read once, with the token only in the request header", async () => {
	process.env.GITHUB_TOKEN = "token-for-the-test";
	const calls: { url: string; headers: Record<string, string> }[] = [];
	globalThis.fetch = async (input, init) => {
		calls.push({ url: String(input), headers: (init?.headers ?? {}) as Record<string, string> });
		return Response.json({ tag_name: "v9.8.7", html_url: `${REPO}/releases/tag/v9.8.7`, assets: [asset("stuarty-desktop-9-8-7.exe"), { name: 5 }] });
	};
	const latestRelease = await fresh();
	const release = await latestRelease();
	await latestRelease();
	assert.equal(calls.length, 1);
	assert.equal(calls[0].url, "https://api.github.com/repos/AlexGalhardo/stuarty/releases/latest");
	assert.equal(calls[0].headers.Authorization, "Bearer token-for-the-test");
	assert.equal(release.version, "9.8.7");
	assert.equal(release.url, `${REPO}/releases/tag/v9.8.7`);
	assert.equal(release.downloads[0].href, `${REPO}/releases/download/v9.8.7/stuarty-desktop-9-8-7.exe`);
	assert.ok(!JSON.stringify(release).includes("token-for-the-test"));
});

test("without a token the request carries no Authorization header", async () => {
	delete process.env.GITHUB_TOKEN;
	let headers: Record<string, string> = {};
	globalThis.fetch = async (_input, init) => {
		headers = (init?.headers ?? {}) as Record<string, string>;
		return Response.json({ tag_name: "v1.0.0", assets: [] });
	};
	await (await fresh())();
	assert.equal(headers.Authorization, undefined);
});
