import { expect, seedProject, test } from "./helpers";

const SRT = "1\n00:00:01,000 --> 00:00:02,500\nHello there\n";

// TASK-0287 (SDD-012 section 6.4): the Captions tab of the material rail reuses
// the caption sections of the inspector; each control sends its own command.
test.describe("library captions tab", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("import subtitles and the track style come from the tab", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("tab", { name: "Captions" }).click();
		const panel = page.getByRole("region", { name: "Captions" });
		await expect(panel).toBeVisible();
		await expect(panel).toContainText("Select a video clip with audio");

		// The first visual track is the target when nothing is selected.
		const edit = page.waitForRequest(
			(r) => r.method() === "POST" && r.url().endsWith("/edit"),
		);
		await panel
			.getByRole("button", { name: "Import subtitles", exact: true })
			.click();
		await panel.locator("[data-subtitles-file]").setInputFiles({
			name: "talk.srt",
			mimeType: "text/plain",
			buffer: Buffer.from(SRT),
		});
		const body = (await edit).postDataJSON() as {
			operations: { op: string; args: { track_id: string } }[];
		};
		expect(body.operations[0]?.op).toBe("caption.import_subtitles");
		expect(body.operations[0]?.args.track_id).toBe(seeded.trackIds[0]);

		// With a clip selected the style is the one of the track of the clip.
		await page.locator(`[data-item-id="${seeded.itemIds[0]}"]`).click();
		await expect(
			panel.getByRole("button", { name: "Caption style" }),
		).toBeVisible();
	});
});
