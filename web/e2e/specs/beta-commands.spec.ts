import type { Page } from "@playwright/test";
import registry from "../../resources/js/generated/commands.json" with {
	type: "json",
};
import {
	audioClip,
	caller,
	expect,
	seedProject,
	test,
	testClip,
} from "./helpers";
import { openInspectorTab, openSection } from "./sections";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

interface BetaCommand {
	name: string;
	summary: string;
	scope: string;
	phase: string;
	args_schema: { properties: Record<string, unknown> };
}

const BETA = (registry as unknown as BetaCommand[]).filter(
	(c) => c.phase === "beta",
);

function recordEdits(page: Page): Posted[] {
	const edits: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(r.postDataJSON());
		}
	});
	return edits;
}

// A notice that an edit leaves can cover the controls below it.
async function dismissNotices(page: Page): Promise<void> {
	const dismiss = page.getByRole("button", { name: "Dismiss", exact: true });
	for (let i = 0; i < 10 && (await dismiss.count()) > 0; i++) {
		// The notices stack over each other, so a pointer click can land on a
		// sibling; the keyboard reaches the button. One may leave on its own
		// between the count and the press.
		await dismiss
			.first()
			.press("Enter", { timeout: 2000 })
			.catch(() => {});
	}
}

async function seed(
	request: Parameters<typeof seedProject>[0],
): Promise<{ id: string; clip: string; audio: string }> {
	const seeded = await seedProject(request, {
		files: [testClip(0), audioClip(0)],
		tracks: 1,
		perTrack: 1,
		clipDuration: "4s",
		audioTracks: 1,
	});
	const call = await caller(request);
	const head = (await call(
		"GET",
		`/projects/${seeded.id}?detail=document`,
	)) as { revision: number };
	const out = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: [
			{
				op: "audio.add",
				args: {
					track_id: seeded.audioTrackIds[0],
					asset_id: seeded.assetIds[1],
					start: "0s",
				},
			},
		],
	})) as { created: string[] };
	return {
		id: seeded.id,
		clip: seeded.itemIds[0] ?? "",
		audio: out.created[0] ?? "",
	};
}

// P3-UI-007 (SDD-012 sections 9.3 and 12.1): the inspector entry points of the
// BETA item commands.
test.describe("beta-commands", () => {
	// Each test seeds a project with a clip and an audio item through the real
	// server and imports two files: well above the default 30 s on a busy machine.
	test.describe.configure({ timeout: 120_000 });

	test("the BETA registry draws the BETA controls", async ({
		page,
		request,
	}) => {
		const seeded = await seed(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${seeded.clip}"]`).click();
		await openInspectorTab(page, "Audio");
		await expect(
			page.getByRole("button", { name: "Silences", exact: true }),
		).toBeVisible();
		await openInspectorTab(page, "Adjust");
		await expect(
			page.getByRole("button", { name: "Filters", exact: true }),
		).toBeVisible();
		await page.locator(`[data-item-id="${seeded.audio}"]`).click();
		await openInspectorTab(page, "Audio");
		await expect(
			page.getByRole("button", { name: "Audio", exact: true }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Ducking", exact: true }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Beat markers", exact: true }),
		).toBeVisible();
	});

	test("filter, blur, ducking, beat markers and fonts each send one command", async ({
		page,
		request,
	}) => {
		const seeded = await seed(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const edits = recordEdits(page);

		await page.locator(`[data-item-id="${seeded.clip}"]`).click();
		await openInspectorTab(page, "Adjust");
		await page.getByLabel("Filter", { exact: true }).selectOption("sepia");
		await expect.poll(() => edits.length).toBe(1);
		const amount = page.getByLabel("Filter amount", { exact: true });
		await amount.focus();
		await amount.press("Home");
		await expect.poll(() => edits.length).toBe(2);
		const blur = page.getByLabel("Blur", { exact: true });
		await blur.focus();
		await blur.press("End");
		await expect.poll(() => edits.length).toBe(3);

		await dismissNotices(page);
		await page.locator(`[data-item-id="${seeded.audio}"]`).click();
		await openInspectorTab(page, "Audio");
		const apply = page.getByRole("button", {
			name: "Duck under speech",
			exact: true,
		});
		await expect(apply).toBeDisabled();
		const speech = page.getByRole("checkbox", { name: "gradient10-0.mp4" });
		await speech.focus();
		await speech.press("Space");
		await expect(speech).toBeChecked();
		await apply.focus();
		await apply.press("Enter");
		await expect.poll(() => edits.length).toBe(4);
		await dismissNotices(page);
		const every = page.getByLabel("Mark every nth beat", { exact: true });
		await every.fill("2");
		await every.press("Enter");
		await page.getByLabel("Marker label", { exact: true }).fill("beat");
		const mark = page.getByRole("button", { name: "Mark beats", exact: true });
		await mark.focus();
		await mark.press("Enter");
		await expect.poll(() => edits.length).toBe(5);

		await page.keyboard.press("Escape");
		await dismissNotices(page);
		await openSection(page, "Fonts");
		await page.getByLabel("Family name", { exact: true }).fill("Brand Sans");
		await page
			.getByLabel("Font file hash (sha256:...)", { exact: true })
			.fill(`sha256:${"ab".repeat(32)}`);
		const addFont = page.getByRole("button", { name: "Add font", exact: true });
		await addFont.focus();
		await addFont.press("Enter");
		await expect.poll(() => edits.length).toBe(6);

		expect(edits.map((e) => e.operations)).toEqual([
			[
				{
					op: "clip.set_filter",
					args: { item_id: seeded.clip, filter: "sepia" },
				},
			],
			[{ op: "clip.set_filter", args: { item_id: seeded.clip, amount: 0 } }],
			[{ op: "clip.set_blur", args: { item_id: seeded.clip, radius: 100 } }],
			[
				{
					op: "audio.duck",
					args: {
						item_id: seeded.audio,
						speech_item_ids: [seeded.clip],
						gain_db: -12,
						attack: "0.2s",
						release: "0.5s",
					},
				},
			],
			[
				{
					op: "audio.mark_beats",
					args: {
						item_id: seeded.audio,
						every: 2,
						replace: true,
						label: "beat",
					},
				},
			],
			[
				{
					op: "font.add",
					args: {
						family: "Brand Sans",
						files: [{ weight: 400, hash: `sha256:${"ab".repeat(32)}` }],
					},
				},
			],
		]);
	});

	test("every BETA command opens a palette form with a control per property and no JSON fallback", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `Beta palette ${Date.now()}`,
		});
		await page.goto(`/editor/${created.project_id as string}`);
		await expect(
			page.getByRole("button", { name: /^Commands \(Ctrl\+K\)/ }),
		).toBeEnabled();
		await page.keyboard.press("Control+k");
		const search = page.getByRole("combobox", { name: "Search commands" });
		for (const command of BETA) {
			await search.fill(command.name);
			await page.locator(`[data-command="${command.name}"]`).click();
			const form = page.locator(`[data-form="${command.name}"]`);
			await expect(form, command.name).toBeVisible();
			for (const property of Object.keys(command.args_schema.properties)) {
				await expect(
					form.locator(`[data-path="args.${property}"]`),
					`${command.name}.${property}`,
				).toHaveCount(1);
			}
			await expect(
				form.locator('[data-fallback="json"]'),
				command.name,
			).toHaveCount(0);
			await page.getByRole("button", { name: "Back to the list" }).click();
		}
	});
});
