import type { APIRequestContext, Page } from "@playwright/test";

import { ensureAccount, signInWithPassword } from "../support/account";
import { caller, expect, home, seedProject, test } from "./helpers";

// Epic 64 (SDD-025 section 19): an agent proposes cuts of a take, a person decides each range, and the capture that
// is on is visible on every page of the editor. Chromium plays the devices as in web-recording.spec.ts. The tests run
// one after the other: one browser upload may be open at a time for a person.
test.describe.configure({ timeout: 240_000, mode: "serial" });

test.use({
	viewport: { width: 1440, height: 900 },
	permissions: ["microphone", "camera"],
	launchOptions: {
		args: [
			"--use-fake-ui-for-media-stream",
			"--use-fake-device-for-media-stream",
			"--auto-select-desktop-capture-source=Entire screen",
		],
	},
});

interface RecordingView {
	recording_id: string;
	state: string;
	review?: string;
	duration_ms: number;
	asset_id?: string;
	proposals?: {
		proposal_id: string;
		state: string;
		ranges: { decision?: string }[];
	}[];
}

type Call = Awaited<ReturnType<typeof caller>>;

async function recordingsOf(
	call: Call,
	project: string,
): Promise<RecordingView[]> {
	const got = (await call("GET", `/projects/${project}/recordings`)) as {
		recordings: RecordingView[];
	};
	return got.recordings;
}

async function openDialog(page: Page, project: string): Promise<void> {
	await ensureAccount(page.request, home);
	await signInWithPassword(page);
	await page.goto(`/editor/${project}`);
	await page.locator("[data-import-open]").click();
	await page.locator("[data-record-open]").click();
	await expect(page.locator("[data-record-step]")).toBeVisible();
}

async function revisionOf(call: Call, project: string): Promise<number> {
	const got = (await call("GET", `/projects/${project}?detail=document`)) as {
		revision: number;
	};
	return got.revision;
}

// The summed duration of the clips of the timeline, in frames.
async function clipFrames(call: Call, project: string): Promise<number> {
	const got = (await call("GET", `/projects/${project}?detail=document`)) as {
		document: { items: Record<string, { type: string; duration: number }> };
	};
	return Object.values(got.document.items)
		.filter((item) => item.type === "clip")
		.reduce((sum, item) => sum + item.duration, 0);
}

// A person records about four seconds in the browser, adds the take and places it on the timeline.
async function placedTake(
	page: Page,
	request: APIRequestContext,
	project: string,
): Promise<string> {
	await openDialog(page, project);
	await page.locator("[data-record-start]").click();
	await expect(page.locator("[data-record-browser-live]")).toBeVisible({
		timeout: 30_000,
	});

	// a capture that is on is visible on the page itself, outside the dialog (SDD-025 section 19.4)
	await expect(page.locator("[data-recording-banner]")).toBeVisible({
		timeout: 20_000,
	});
	await expect(page.locator("[data-recording-banner]")).toContainText(
		"Recording the screen",
	);

	await expect(page.locator("[data-record-clock]")).toContainText(
		"Elapsed 0:04",
		{ timeout: 45_000 },
	);
	await page.locator("[data-record-stop]").click();
	await expect(page.locator("[data-record-notice]")).toBeVisible({
		timeout: 90_000,
	});
	await page.locator("[data-record-add]").click();
	await expect(page.locator("[data-record-step]")).toHaveAttribute(
		"data-record-step",
		"added",
	);

	const call = await caller(request);
	const [added] = await recordingsOf(call, project);
	const assetId = added?.asset_id ?? "";
	expect(assetId).toMatch(/^ast_/);
	// the asset of the take is in the library once its ingest is done
	await expect
		.poll(
			async () => {
				const media = (await call("GET", `/projects/${project}/media`)) as {
					assets: { asset_id: string; state: string }[];
				};
				return media.assets.find((a) => a.asset_id === assetId)?.state;
			},
			{ timeout: 60_000 },
		)
		.toBe("ready");
	await page.locator("[data-record-place]").click();
	await expect(page.getByText("Placed on the timeline.")).toBeVisible();

	// once the take is over, the banner goes away
	await expect(page.locator("[data-recording-banner]")).toHaveCount(0, {
		timeout: 20_000,
	});

	const [rec] = await recordingsOf(call, project);
	if (rec === undefined) throw new Error("the recording is not listed");
	return rec.recording_id;
}

test.describe("an agent proposes cuts and a person decides", () => {
	test("each range is decided by the person, one batch cuts the accepted ones, and the banner shows a capture @chromium-only", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 0, assets: 1 });
		const take = await placedTake(page, request, seeded.id);
		const call = await caller(request, "claude-code");

		const revision = await revisionOf(call, seeded.id);
		const frames = await clipFrames(call, seeded.id);

		// the agent proposes two cuts: nothing on the timeline moves
		const proposed = (await call("POST", `/recordings/${take}/proposals`, {
			ranges: [
				{ from_ms: 500, to_ms: 1000, reason: "a long pause" },
				{ from_ms: 2000, to_ms: 2500, reason: "a repeated word" },
			],
		})) as RecordingView;
		const proposal = proposed.proposals?.[0];
		expect(proposal?.state).toBe("pending");
		expect(await revisionOf(call, seeded.id)).toBe(revision);
		expect(await clipFrames(call, seeded.id)).toBe(frames);

		// the agent cannot decide its own proposal
		const refused = await request.post(
			`/api/v1/recordings/${take}/proposals/${proposal?.proposal_id}/decide`,
			{
				headers: await agentHeaders(),
				data: { accept: [true, true] },
			},
		);
		expect(refused.status()).toBe(403);
		expect(((await refused.json()) as { code: string }).code).toBe(
			"E_PRIVILEGED_SESSION",
		);
		expect(await revisionOf(call, seeded.id)).toBe(revision);

		// the person sees both ranges with their reasons, keeps the second one and applies
		await page.reload();
		await page.locator("[data-import-open]").click();
		await page.locator("[data-record-open]").click();
		const card = page.locator(
			`[data-record-proposal="${proposal?.proposal_id}"]`,
		);
		await expect(card).toBeVisible();
		await expect(card.locator("[data-record-range]")).toHaveCount(2);
		await expect(card).toContainText("a long pause");
		await expect(card).toContainText("2 of 2 ranges will be cut");
		await card.locator('[data-record-range-reject="1"]').click();
		await expect(card).toContainText("1 of 2 ranges will be cut");
		await card.locator("[data-record-proposal-apply]").click();
		await expect(page.locator("[data-record-decided]")).toContainText(
			"1 of 2 ranges were cut in one batch",
		);

		// one batch: one revision more, and the take is half a second shorter
		expect(await revisionOf(call, seeded.id)).toBe(revision + 1);
		expect(frames - (await clipFrames(call, seeded.id))).toBeGreaterThan(10);
		const [after] = await recordingsOf(call, seeded.id);
		expect(after?.proposals?.[0]?.state).toBe("accepted");
		expect(after?.proposals?.[0]?.ranges.map((r) => r.decision)).toEqual([
			"accepted",
			"rejected",
		]);
		// nothing waits for a decision any more
		await expect(page.locator("[data-record-proposals]")).toHaveCount(0);
	});

	test("an agent that prepares a recording cannot start it", async ({
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 0, assets: 1 });
		const call = await caller(request, "claude-code");

		// native capture does not exist in a browser test run, so the preparation is the agent's whole reach
		const prepared = await request.post("/api/v1/recordings/prepare", {
			headers: await agentHeaders(),
			data: {
				project_id: seeded.id,
				source: { kind: "display", id: "d1" },
				notes: "Say the hook first",
			},
		});
		expect(prepared.status()).toBe(200);
		const rec = (await prepared.json()) as RecordingView;
		expect(rec.state).toBe("prepared");

		const started = await request.post("/api/v1/recordings", {
			headers: await agentHeaders(),
			data: { recording_id: rec.recording_id },
		});
		expect(started.status()).toBe(403);
		expect(((await started.json()) as { code: string }).code).toBe(
			"E_PRIVILEGED_SESSION",
		);
		const [still] = await recordingsOf(call, seeded.id);
		expect(still?.state).toBe("prepared");
	});
});

async function agentHeaders(): Promise<Record<string, string>> {
	const { readFile } = await import("node:fs/promises");
	const { join } = await import("node:path");
	const token = (await readFile(join(home, "token"), "utf8")).trim();
	return { Authorization: `Bearer ${token}`, "Stuarty-Client": "claude-code" };
}
