import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const edits: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(r.postDataJSON() as Posted);
		}
	});
	return edits;
}

const SRT =
	"1\n00:00:01,000 --> 00:00:02,500\nHello there\n\n2\n00:00:03,000 --> 00:00:04,000\nBye\n";

// TASK-0254, SDD-012 section 12.1 (NEW-32a): the track panel of a visual track
// picks an SRT or WebVTT file, the page reads its text and sends it in one
// caption.import_subtitles; the core parses it and a refusal shows as received.
test.describe("import subtitles", () => {
	test("a picked .srt file becomes captions and a bad file shows the error of the core", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		const trackId = seeded.trackIds[0] ?? "";
		await page.goto(`/editor/${seeded.id}`);
		await page.locator("[data-header-for]").click();
		await page
			.getByRole("button", { name: "Import subtitles", exact: true })
			.click();
		const edits = recordEdits(page);
		const picker = page.locator("[data-subtitles-file]");

		await picker.setInputFiles({
			name: "talk.srt",
			mimeType: "text/plain",
			buffer: Buffer.from(SRT),
		});
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "caption.import_subtitles",
				args: { track_id: trackId, text: SRT, format: "srt" },
			},
		]);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);

		await picker.setInputFiles({
			name: "bad.vtt",
			mimeType: "text/plain",
			buffer: Buffer.from("this is not a subtitle file"),
		});
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]?.args).toMatchObject({ format: "vtt" });
		await expect(page.getByRole("alert")).toContainText(/no cue with text/i);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
	});
});
