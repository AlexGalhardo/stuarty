import { audioClip, caller, expect, test } from "./helpers";

// Epic 99 task 4 (SDD-011 sections 14.11 and 14.12): the tag filter of a library is a picker that lists the tags in
// use with their counts, and the sounds are filtered by kind and the kind of a sound is changed from its detail.
test.describe.configure({ timeout: 120_000 });

test.describe("sound kind and tag picker", () => {
	test("filters the sounds by kind, changes the kind from the detail and counts the tags", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const stamp = Date.now();
		const tag = `kind-${stamp}`;
		const project = (
			(await call("POST", "/projects", { name: `Sound kind ${stamp}` })) as {
				project_id: string;
			}
		).project_id;
		const imported = (await call("POST", `/projects/${project}/media`, {
			files: [{ path: audioClip(61) }],
			wait_ms: 20_000,
			info: { tags: [tag], sound_kind: "effect" },
		})) as { assets: { hash: string }[] };
		const hash = imported.assets[0]?.hash;
		expect(hash).toMatch(/^sha256:[0-9a-f]{64}$/);
		await call("PUT", `/assets/${hash}`, { sound_kind: "effect" });

		await page.goto("/sound-assets");
		const row = page.locator(`[data-asset-hash="${hash}"]`);
		await expect(row.getByTestId("sound-kind-badge")).toHaveText("Effect");

		// The tag picker lists the tag with its count.
		await page.getByRole("combobox", { name: "Tag" }).click();
		await expect(
			page.getByRole("option", { name: `${tag} (1 file)` }),
		).toBeVisible();
		await page.keyboard.press("Escape");

		// Only the kind chosen stays.
		await page.getByRole("combobox", { name: "Kind" }).click();
		await page.getByRole("option", { name: "Music" }).click();
		await expect(row).toHaveCount(0);
		await page.getByRole("combobox", { name: "Kind" }).click();
		await page.getByRole("option", { name: "Effect" }).click();
		await expect(row).toBeVisible();

		// The kind is changed from the detail, and the row follows what the service answered.
		await row.getByRole("button").first().click();
		const detail = page.getByTestId("asset-detail");
		await detail.getByRole("combobox", { name: "What this sound is" }).click();
		await page.getByRole("option", { name: "Voice" }).click();
		await expect(detail.getByRole("status")).toHaveText("Saved.");
		const stored = (await call("GET", `/assets/${hash}`)) as {
			sound_kind?: string;
		};
		expect(stored.sound_kind).toBe("voice");
		await expect(row).toHaveCount(0);

		await call("PUT", `/assets/${hash}`, { sound_kind: "" });
		await call("POST", `/assets/${hash}/tags`, { add: [], remove: [tag] });
	});
});
