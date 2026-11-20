import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { bytes, duration, limitRows, networkPage, networksOverview, readNetworks, type NetworkDoc } from "./network-reference.ts";

const siteDir = path.join(import.meta.dirname, "..");
const root = path.join(siteDir, "..");
const networks = readNetworks(siteDir);

test("sizes and durations are written the way a person reads them", () => {
	assert.equal(bytes(8_000_000_000), "8 GB");
	assert.equal(bytes(300_000_000), "300 MB");
	assert.equal(duration(900_000), "15 min");
	assert.equal(duration(90_000), "90 s");
	assert.equal(duration(3_000), "3 s");
});

test("every network has the page it was read on and the date, on its own page and on the overview", () => {
	assert.deepEqual(
		networks.map((n) => n.network),
		["youtube", "instagram", "facebook", "x", "tiktok"],
	);
	const overview = networksOverview(networks);
	for (const n of networks) {
		const text = networkPage(n, undefined);
		assert.ok(text.includes(n.limits.source), n.network);
		assert.ok(text.includes(`read on ${n.limits.read}`), n.network);
		assert.ok(overview.includes(n.limits.read), n.network);
	}
});

test("a limit the network does not state is not invented", () => {
	const x = networks.find((n) => n.network === "x") as NetworkDoc;
	const rows = limitRows(x).map((r) => r[0]);
	assert.ok(!rows.includes("Caption"));
	assert.ok(rows.includes("Longest video"));
});

test("the setup steps of the developer app come before the limits", () => {
	const n = networks[0] as NetworkDoc;
	const text = networkPage(n, "1. Create the app.");
	assert.ok(text.indexOf("Create the app.") < text.indexOf("## Limits"));
});

test("every operation of the registry has a generated reference page", () => {
	const registry = JSON.parse(fs.readFileSync(path.join(root, "core", "schema", "commands.json"), "utf8")) as { name: string; internal?: boolean }[];
	const generated = path.join(siteDir, "content", "generated");
	const have = new Set<string>();
	const walk = (dir: string) => {
		for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
			if (e.isDirectory()) walk(path.join(dir, e.name));
			else have.add(e.name.replace(/\.mdx$/, ""));
		}
	};
	walk(path.join(generated, "api"));
	walk(path.join(generated, "mcp"));
	for (const c of registry.filter((c) => !c.internal)) {
		assert.ok(have.has(c.name.replace(/:/g, ".")) || have.has(c.name), `${c.name} has no page`);
	}
});

test("the price of a post on X is generated with its source and date", () => {
	const x = networks.find((n) => n.network === "x") as NetworkDoc;
	const text = networkPage(x, undefined);
	assert.match(text, /0\.015 USD for a post and 0\.2 USD for a post with a link/);
	assert.ok(text.includes(`on ${x.price?.read}`));
	assert.ok(!networkPage(networks[0] as NetworkDoc, undefined).includes("The price of a post"));
});
