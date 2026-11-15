import { expect, test } from "@playwright/test";

import { PENDING_SERVER } from "./helpers";

// P1-UI-004 asserts that need a real server: the scenarios of SDD-012 section
// 20.1. The unit tests of resources/js/editor/commands and export cover the
// same gestures against the fixture server.
test.describe("thin-client-no-optimistic", () => {
	test.skip(true, PENDING_SERVER);

	test("one gesture sends exactly one edit request and the item moves only on the answer", async ({
		page,
	}) => {
		const edits: string[] = [];
		page.on("request", (request) => {
			if (request.url().endsWith("/edit")) edits.push(request.url());
		});
		await page.goto("/editor/prj_0000000000");
		await page.getByRole("button", { name: "Split" }).click();
		expect(edits).toHaveLength(1);
	});
});

test.describe("overlap-snapback", () => {
	test.skip(true, PENDING_SERVER);

	test("an overlap leaves the item in place and shows the message of the core", async ({
		page,
	}) => {
		await page.goto("/editor/prj_0000000000");
		await page.getByRole("button", { name: "Add to timeline" }).first().click();
		await expect(page.getByRole("alert")).toContainText(
			"Overlaps another item",
		);
	});
});

test.describe("command-round-trip", () => {
	test.skip(true, PENDING_SERVER);

	test("the round trip of a command is at most 50 ms at the 95th percentile on the 200-item fixture (NFR-001)", async () => {
		test.fixme(true, "needs the 200-item project and the server (P1-SRV-005)");
	});
});

test.describe("export-dialog", () => {
	test.skip(true, PENDING_SERVER);

	test("lint, start, progress, then the path and a download link", async ({
		page,
	}) => {
		await page.goto("/editor/prj_0000000000");
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();
		await expect(page.getByRole("link", { name: "Download" })).toBeVisible({
			timeout: 60_000,
		});
	});
});
