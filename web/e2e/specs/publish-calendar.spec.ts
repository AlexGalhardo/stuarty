import { randomBytes } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import { readFile } from "node:fs/promises";
import {
	createServer,
	type IncomingMessage,
	type Server,
	type ServerResponse,
} from "node:http";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { APIRequestContext, Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { listenWhenFree, SOCIAL_PORT } from "../support/social";
import { caller, expect, home, test } from "./helpers";

// ADR-0046, ADR-0032, epic 20 (SDD-020 sections 23 to 31): a creator plans a post on the calendar, a person
// approves it, and it goes out at its time. The network is the fake of this spec on loopback, which the server of
// the suite is told about with STUARTY_TEST_SOCIAL_URL; it records what it receives, so the spec can prove that
// nothing was sent before the approval and that a post was sent once.
test.describe.configure({ mode: "serial" });

const CLIENT_SECRET = "GOCSPX-e2e-client-secret-CANARY";

interface FakeNetwork {
	received: string[];
	uploads(): number;
	close(): Promise<void>;
}

function readBody(request: IncomingMessage): Promise<string> {
	return new Promise((resolve) => {
		let data = "";
		request.on("data", (chunk) => {
			data += String(chunk);
		});
		request.on("end", () => resolve(data));
	});
}

// The fake network of the suite. The consent screen redirects straight back with a code; an upload is recorded.
// The youtube-connect spec listens on the same port, so a bind that finds it taken waits for it to be free.
async function fakeNetwork(): Promise<FakeNetwork> {
	const received: string[] = [];
	let uploads = 0;
	const server: Server = createServer(
		(request: IncomingMessage, response: ServerResponse) => {
			void (async () => {
				const url = new URL(
					request.url ?? "/",
					`http://127.0.0.1:${SOCIAL_PORT}`,
				);
				received.push(`${request.method} ${url.pathname}`);
				const body = await readBody(request);
				response.setHeader("Content-Type", "application/json");
				if (url.pathname === "/authorize") {
					const target = new URL(url.searchParams.get("redirect_uri") ?? "");
					target.searchParams.set("code", "e2e-code");
					target.searchParams.set("state", url.searchParams.get("state") ?? "");
					response.writeHead(302, { Location: target.toString() });
					response.end();
				} else if (url.pathname === "/token") {
					const form = new URLSearchParams(body);
					if (form.get("client_secret") !== CLIENT_SECRET) {
						response.writeHead(400);
						response.end(JSON.stringify({ error: "invalid_client" }));
						return;
					}
					response.end(
						JSON.stringify({
							access_token: "e2e-access",
							refresh_token: "e2e-refresh",
							expires_in: 3600,
						}),
					);
				} else if (url.pathname === "/account") {
					response.end(JSON.stringify({ id: "UC-E2E", title: "E2E channel" }));
				} else if (url.pathname === "/videos" && request.method === "POST") {
					uploads += 1;
					const id = `cal${uploads}`;
					response.end(
						JSON.stringify({
							id,
							url: `https://fake.example/watch/${id}`,
							state: "published",
						}),
					);
				} else {
					response.end("{}");
				}
			})();
		},
	);
	await listenWhenFree(server);
	return {
		received,
		uploads: () => received.filter((r) => r === "POST /videos").length,
		close: () => new Promise((resolve) => server.close(() => resolve())),
	};
}

const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

function newId(prefix: string): string {
	let out = "";
	for (const byte of randomBytes(10)) out += ALPHABET[byte % ALPHABET.length];
	return `${prefix}_${out}`;
}

// A succeeded export of the project and the file it made: the product runs the real export in other specs.
function seedExport(project: string): string {
	const id = newId("job");
	const dir = join(home, "exports", project);
	mkdirSync(dir, { recursive: true });
	const data = Buffer.from("e2e calendar video bytes");
	writeFileSync(join(dir, `${id}.mp4`), data);
	const db = new DatabaseSync(join(home, "db.sqlite"));
	try {
		db.exec("PRAGMA busy_timeout = 10000");
		const now = Date.now();
		db.prepare(
			`INSERT INTO jobs (id, kind, project_id, actor, state, progress, message, input, result, created_ms, updated_ms, started_ms, finished_ms)
 VALUES (?, 'export', ?, 'local:ui', 'succeeded', 1, '', '{}', ?, ?, ?, ?, ?)`,
		).run(
			id,
			project,
			JSON.stringify({ export: { format: "mp4", bytes: data.length } }),
			now,
			now,
			now,
			now,
		);
	} finally {
		db.close();
	}
	return id;
}

async function signInAsPerson(page: Page): Promise<void> {
	const launch = await page.request.post("/api/v1/system/launch", {
		headers: {
			Authorization: `Bearer ${(await readFile(join(home, "token"), "utf8")).trim()}`,
			"Stuarty-Launcher": LAUNCHER_SECRET,
		},
	});
	expect(launch.ok()).toBe(true);
	const { url } = (await launch.json()) as { url: string };
	await page.goto(url);
}

// The signed-in account of the fake network, made the way a person does it in Connections.
async function connectAccount(page: Page): Promise<void> {
	await page.goto("/connections");
	await page.getByRole("button", { name: "Set up Fake network" }).click();
	await page.getByLabel("Client id").fill("e2e-client-id");
	await page.getByLabel("Client secret").fill(CLIENT_SECRET);
	await page.getByRole("button", { name: "Save and sign in" }).click();
	await expect(page.locator("[data-signed-in]")).toContainText("E2E channel");
}

interface Target {
	id: string;
	state: string;
	scheduled_at: string | null;
	url?: string;
	error?: { message: string; hint?: string };
}

interface Publication {
	id: string;
	targets: Target[];
}

async function publicationOf(
	api: Awaited<ReturnType<typeof caller>>,
	id: string,
): Promise<Publication> {
	return (await api("GET", `/publications/${id}`)) as Publication;
}

// A date and time as a datetime-local field holds it, in the zone of the machine that runs the browser.
function localField(date: Date): string {
	const pad = (n: number): string => String(n).padStart(2, "0");
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

async function cleanUp(
	request: APIRequestContext,
	network: FakeNetwork,
): Promise<void> {
	const api = await caller(request);
	const list = (await api("GET", "/connections")) as {
		connections: { id: string; kind: string }[];
	};
	for (const connection of list.connections.filter((c) => c.kind === "fake")) {
		await api("DELETE", `/connections/${connection.id}`);
	}
	await network.close();
}

// @chromium-only: the fake network listens on the one port the server of the suite is told about; one engine is
// enough for a flow of the product.
test.describe("publishing calendar @chromium-only", () => {
	test("plan a post, approve it, and it is published at its time, once", async ({
		page,
		request,
	}) => {
		test.setTimeout(240_000);
		const network = await fakeNetwork();
		try {
			await signInAsPerson(page);
			await connectAccount(page);

			const api = await caller(request);
			const made = (await api("POST", "/projects", {
				name: "Calendar e2e",
				preset: "9:16",
			})) as { project_id: string };
			seedExport(made.project_id);

			await page.goto("/publish");
			await expect(
				page.getByRole("heading", { name: "Publish", level: 1 }),
			).toBeVisible();
			await expect(page.getByTestId("calendar-month")).toBeVisible();
			await page.getByTestId("publish-new").click();

			const dialog = page.getByTestId("publication-editor");
			await dialog.getByLabel("Project").selectOption(made.project_id);
			await dialog.getByLabel("Exported video").selectOption({ index: 1 });
			await dialog.getByLabel("Title", { exact: true }).fill("Calendar post");
			await dialog.getByLabel("Caption").fill("A caption for the week");
			await dialog.getByRole("button", { name: "Add an account" }).click();
			const panel = dialog.getByTestId("target-panel");
			await panel.getByLabel("Account", { exact: true }).selectOption({
				index: 1,
			});
			// The next whole minute that is at least half a minute away.
			const at = new Date(Date.now() + 30_000);
			at.setSeconds(0, 0);
			at.setMinutes(at.getMinutes() + 1);
			await panel.getByLabel("Date and time").fill(localField(at));
			await panel.getByLabel("No", { exact: true }).check();
			await dialog.getByRole("button", { name: "Save", exact: true }).click();

			// Saved as a draft: nothing is asked, nothing is sent.
			await expect(panel.getByTestId("target-state")).toHaveText("Draft");
			expect(network.uploads()).toBe(0);

			await panel.getByTestId("target-schedule").click();
			await expect(panel.getByTestId("target-state")).toHaveText(
				"Waiting for approval",
			);
			await expect(panel.getByTestId("target-waits")).toBeVisible();
			expect(network.uploads()).toBe(0);
			await page.keyboard.press("Escape");
			await expect(dialog).toHaveCount(0);

			// The post is on today's cell of the calendar, in words as well as in colour.
			// Only the chip of this post: other specs (x-tiktok) plan publications in the same workspace at the
			// same time.
			const chip = page
				.getByTestId("target-chip")
				.filter({ hasText: "Calendar post" });
			await expect(chip).toHaveCount(1);
			await expect(chip).toContainText("Calendar post");
			await expect(chip).toContainText("Waiting for approval");

			const publications = (await api(
				"GET",
				`/publications?project_id=${made.project_id}`,
			)) as { publications: Publication[] };
			const pub = publications.publications[0];
			expect(pub).toBeDefined();
			const id = pub?.id ?? "";

			// A person approves it in the queue.
			await page.goto("/approvals");
			const item = page
				.locator("[data-approval-id]")
				.filter({ hasText: "Calendar post" });
			await expect(item.getByTestId("approval-summary")).toContainText(
				"Calendar post",
			);
			await item.getByRole("button", { name: "Approve" }).click();
			await expect(item).toHaveCount(0);
			expect(network.uploads()).toBe(0);

			await page.goto("/publish");
			await expect(chip).toContainText("Scheduled");

			// At its time the scheduler sends it, once.
			await expect
				.poll(async () => (await publicationOf(api, id)).targets[0]?.state, {
					timeout: 150_000,
				})
				.toBe("published");
			expect(network.uploads()).toBe(1);

			await expect(chip).toContainText("Published", {
				timeout: 30_000,
			});
			await page.getByTestId("publish-mode-list").click();
			const row = page.locator(
				`[data-testid="publish-row"][data-target-id="${pub?.targets[0]?.id}"]`,
			);
			await expect(row).toHaveCount(1);
			await expect(row.getByTestId("target-url")).toHaveAttribute(
				"href",
				"https://fake.example/watch/cal1",
			);
			// Further passes of the scheduler send nothing more.
			expect(network.uploads()).toBe(1);
		} finally {
			await cleanUp(request, network);
		}
	});

	// D-111, VV-280: the export of an image, a cover made from a template for instance, is the thumbnail of a post.
	// The person pastes the job the image editor named; the service checks it, so an unknown one is refused.
	test("the export of an image is the thumbnail of a publication", async ({
		page,
		request,
	}) => {
		const network = await fakeNetwork();
		try {
			await signInAsPerson(page);
			await connectAccount(page);

			const api = await caller(request);
			const made = (await api("POST", "/projects", {
				name: "Calendar cover",
				preset: "9:16",
			})) as { project_id: string };
			seedExport(made.project_id);
			const image = (await api("POST", "/images", {
				name: "Calendar cover",
				preset: "video_thumbnail",
			})) as { image_id: string };
			const cover = (await api("POST", `/images/${image.image_id}/export`, {
				format: "png",
			})) as { job_id: string };
			expect(cover.job_id).toMatch(/^job_/);

			await page.goto("/publish");
			await page.getByTestId("publish-new").click();
			const dialog = page.getByTestId("publication-editor");
			await dialog.getByLabel("Project").selectOption(made.project_id);
			await dialog.getByLabel("Exported video").selectOption({ index: 1 });
			await dialog.getByLabel("Title", { exact: true }).fill("Cover post");
			const field = dialog.getByLabel(/^Or the export of an image/);
			await field.fill("job_0000000000");
			await dialog.getByRole("button", { name: "Add an account" }).click();
			const panel = dialog.getByTestId("target-panel");
			await panel.getByLabel("Account", { exact: true }).selectOption({
				index: 1,
			});
			const at = new Date(Date.now() + 2 * 86_400_000);
			at.setHours(9, 0, 0, 0);
			await panel.getByLabel("Date and time").fill(localField(at));
			await panel.getByLabel("No", { exact: true }).check();
			await dialog.getByRole("button", { name: "Save", exact: true }).click();
			await expect(dialog.getByRole("alert")).toContainText(
				"not a job of this project",
			);

			await field.fill(cover.job_id);
			await dialog.getByRole("button", { name: "Save", exact: true }).click();
			await expect(panel.getByTestId("target-state")).toHaveText("Draft");
			await expect(dialog.getByRole("alert")).toHaveCount(0);

			const publications = (await api(
				"GET",
				`/publications?project_id=${made.project_id}`,
			)) as { publications: (Publication & { thumbnail?: string })[] };
			expect(publications.publications[0]?.thumbnail).toBe(cover.job_id);
			expect(network.uploads()).toBe(0);
		} finally {
			await cleanUp(request, network);
		}
	});

	test("the keyboard moves a post: Space takes it, arrows move it, Enter drops it", async ({
		page,
		request,
	}) => {
		const network = await fakeNetwork();
		try {
			await signInAsPerson(page);
			await connectAccount(page);

			const api = await caller(request);
			const made = (await api("POST", "/projects", {
				name: "Calendar keyboard",
				preset: "9:16",
			})) as { project_id: string };
			const exportJob = seedExport(made.project_id);
			const accounts = (await api("GET", "/social/accounts")) as {
				accounts: { connection_id: string }[];
			};
			const start = new Date();
			start.setDate(start.getDate() + 3);
			start.setHours(9, 0, 0, 0);
			const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
			const created = (await api("POST", "/publications", {
				project_id: made.project_id,
				title: "Keyboard post",
				export: exportJob,
				targets: [
					{
						connection_id: accounts.accounts[0]?.connection_id,
						at: localField(start),
						zone,
						fields: { made_for_kids: false },
					},
				],
			})) as Publication;
			const before = created.targets[0]?.scheduled_at ?? "";

			await page.goto("/publish");
			await page.getByTestId("publish-mode-week").click();
			// Look at the week that holds the post.
			const chip = page.locator(
				`[data-testid="target-chip"][data-target-id="${created.targets[0]?.id}"]`,
			);
			for (let i = 0; i < 2; i += 1) {
				if ((await chip.count()) > 0) break;
				await page.getByRole("button", { name: "Next" }).click();
			}
			await expect(chip).toHaveCount(1);
			await chip.focus();
			// Escape lets a post go without moving it.
			await page.keyboard.press("Space");
			await page.keyboard.press("ArrowLeft");
			await page.keyboard.press("Escape");
			await expect(chip).toHaveAttribute("aria-pressed", "false");
			expect(
				(await publicationOf(api, created.id)).targets[0]?.scheduled_at,
			).toBe(before);
			await page.keyboard.press("Space");
			await expect(chip).toHaveAttribute("aria-pressed", "true");
			await page.keyboard.press("ArrowRight");
			await page.keyboard.press("ArrowDown");
			await expect(page.getByRole("status").first()).toContainText("Moving to");
			await page.keyboard.press("Enter");

			// The service moved it by a day and an hour, and asked for approval again.
			await expect
				.poll(async () => {
					const moved = (await publicationOf(api, created.id)).targets[0];
					return moved?.scheduled_at ?? "";
				})
				.not.toBe(before);
			const moved = (await publicationOf(api, created.id)).targets[0];
			const delta =
				new Date(moved?.scheduled_at ?? "").getTime() -
				new Date(before).getTime();
			const hour = 3_600_000;
			// a day and an hour, give or take the hour a change of the clock may add
			expect([25 * hour, 24 * hour, 26 * hour]).toContain(delta);
			expect(moved?.state).toBe("waiting_approval");
			expect(network.uploads()).toBe(0);
		} finally {
			await cleanUp(request, network);
		}
	});

	test("the calendar speaks Portuguese", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await signInAsPerson(page);
		await page.goto("/publish");
		await expect(
			page.getByRole("heading", { name: "Publicar", level: 1 }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Nova publicação" }),
		).toBeVisible();
		await expect(page.getByTestId("publish-mode-week")).toHaveText("Semana");
		await page.getByTestId("publish-mode-list").click();
		await expect(
			page.getByTestId("publish-filter-state").locator("option").first(),
		).toHaveText("Todos");
	});

	// Epic 21 (SDD-012 section 21.3 item 7): in the desktop shell, the first post a person schedules brings the
	// question about the background mode, and only the first. The shell is faked by a bridge that keeps the answer
	// and counts how often the page read it. It runs last: the list of the first test counts the publications.
	test("in the desktop shell the first schedule asks about the background mode, once", async ({
		page,
		request,
	}) => {
		const network = await fakeNetwork();
		const made: Publication[] = [];
		const api = await caller(request);
		try {
			await page.addInitScript(() => {
				const w = window as unknown as Record<string, unknown>;
				let held = {
					keep_running: false,
					start_at_sign_in: false,
					asked: false,
				};
				const sent: unknown[] = [];
				let reads = 0;
				w.__background = { sent, reads: () => reads };
				w.vukemDesktop = {
					getBackground: () => {
						reads += 1;
						return Promise.resolve({ ...held });
					},
					setBackground: (s: typeof held) => {
						sent.push(s);
						held = { ...s };
						return Promise.resolve({ ...held });
					},
				};
			});
			const shell = (): Promise<{ sent: unknown[]; reads: number }> =>
				page.evaluate(() => {
					const b = (
						window as unknown as {
							__background: { sent: unknown[]; reads(): number };
						}
					).__background;
					return { sent: b.sent, reads: b.reads() };
				});

			await signInAsPerson(page);
			await connectAccount(page);
			const project = (await api("POST", "/projects", {
				name: "Calendar background",
				preset: "9:16",
			})) as { project_id: string };
			const exportJob = seedExport(project.project_id);
			const accounts = (await api("GET", "/social/accounts")) as {
				accounts: { connection_id: string }[];
			};
			const at = new Date();
			at.setDate(at.getDate() + 5);
			at.setHours(10, 0, 0, 0);
			const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
			for (const title of ["Background first", "Background second"]) {
				made.push(
					(await api("POST", "/publications", {
						project_id: project.project_id,
						title,
						export: exportJob,
						targets: [
							{
								connection_id: accounts.accounts[0]?.connection_id,
								at: localField(at),
								zone,
								fields: { made_for_kids: false },
							},
						],
					})) as Publication,
				);
			}

			await page.goto("/publish");
			await page.getByTestId("publish-mode-list").click();
			const editor = page.getByTestId("publication-editor");
			const schedule = async (publication: Publication): Promise<void> => {
				await page
					.locator(
						`[data-testid="publish-row"][data-target-id="${publication.targets[0]?.id}"]`,
					)
					.getByRole("button", { name: "Open" })
					.click();
				const panel = editor.getByTestId("target-panel");
				await panel.getByTestId("target-schedule").click();
				await expect(panel.getByTestId("target-state")).toHaveText(
					"Waiting for approval",
				);
			};

			const [first, second] = made;
			if (first === undefined || second === undefined)
				throw new Error("no posts");
			await schedule(first);
			const ask = page.getByTestId("background-ask");
			await expect(ask).toBeVisible();
			await expect(ask).toContainText(
				"only if the computer is on and you are signed in",
			);
			await expect(ask).toContainText("waits for your decision");
			await ask.getByTestId("background-ask-yes").click();
			await expect(ask).toHaveCount(0);
			expect((await shell()).sent).toEqual([
				{ keep_running: true, start_at_sign_in: false, asked: true },
			]);
			await page.keyboard.press("Escape");
			await expect(editor).toHaveCount(0);

			// Answered once, the next schedule reads the answer and asks nothing.
			await schedule(second);
			await expect.poll(async () => (await shell()).reads).toBe(2);
			await expect(ask).toHaveCount(0);
			expect((await shell()).sent).toHaveLength(1);
			expect(network.uploads()).toBe(0);
		} finally {
			// The posts wait for an approval nobody gives: cancelled, they leave the queue.
			for (const publication of made) {
				await api("POST", `/publications/${publication.id}/cancel`, {});
			}
			await cleanUp(request, network);
		}
	});
});
