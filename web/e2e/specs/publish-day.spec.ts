import { randomBytes } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import {
	createServer,
	type IncomingMessage,
	type Server,
	type ServerResponse,
} from "node:http";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { APIRequestContext, Page } from "@playwright/test";
import { ensureAccount, signInWithPassword } from "../support/account";
import { listenWhenFree, SOCIAL_PORT } from "../support/social";
import { caller, expect, home, test } from "./helpers";

// D-146, epic 97 (SDD-020 section 30A): a creator opens one day of the publishing calendar and plans it there. They
// create a post by clicking an hour, adjust it in place, write a note and choose a theme, mark a quiet day that an agent
// cannot schedule on and that a person confirms, keep a weekly slot, copy a post to another day, jump to a date and take
// the plan as an .ics file. The days are in March 2027, far from every other spec of the shared workspace; the network is
// the fake of the suite, which records what it receives so the spec can prove that nothing was sent.
test.describe.configure({ mode: "serial" });

const CLIENT_SECRET = "GOCSPX-e2e-client-secret-CANARY";
const DAY = "2027-03-02"; // a Tuesday
const NEXT = "2027-03-09";

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

async function fakeNetwork(): Promise<FakeNetwork> {
	const received: string[] = [];
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

function seedExport(project: string): string {
	const id = newId("job");
	const dir = join(home, "exports", project);
	mkdirSync(dir, { recursive: true });
	const data = Buffer.from("e2e day video bytes");
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
	await ensureAccount(page.request, home);
	await signInWithPassword(page);
	await page.goto("/");
}

async function connectAccount(page: Page): Promise<void> {
	await page.goto("/connections");
	await page.getByRole("button", { name: "Set up Fake network" }).click();
	await page.getByLabel("Client id").fill("e2e-client-id");
	await page.getByLabel("Client secret").fill(CLIENT_SECRET);
	await page.getByRole("button", { name: "Save and sign in" }).click();
	await expect(page.locator("[data-signed-in]")).toContainText("E2E channel");
}

type Api = Awaited<ReturnType<typeof caller>>;

interface Target {
	id: string;
	state: string;
	scheduled_at: string | null;
	fields: { description?: string };
}

interface Publication {
	id: string;
	title: string;
	targets: Target[];
}

async function publicationOf(api: Api, id: string): Promise<Publication> {
	return (await api("GET", `/publications/${id}`)) as unknown as Publication;
}

async function goToDate(page: Page, date: string): Promise<void> {
	await page.getByTestId("mini-go").fill(date);
	await page.getByTestId("mini-go-submit").click();
	await expect(page.getByTestId("calendar-day-view")).toHaveAttribute(
		"data-date",
		date,
	);
}

function hour(page: Page, h: number) {
	return page.locator(`[data-testid="day-hour"][data-hour="${h}"]`);
}

// Take the posts of the run out of the queue, remove the days it marked and the slots it made, and the connection.
async function cleanUp(
	request: APIRequestContext,
	network: FakeNetwork,
	ids: string[],
): Promise<void> {
	const api = await caller(request);
	for (const id of ids) {
		await api("POST", `/publications/${id}/cancel`, {}).catch(() => undefined);
	}
	const plan = (await api(
		"GET",
		`/calendar/plan?from=${DAY}&to=${NEXT}&zone=UTC`,
	).catch(() => ({ rules: [] }))) as { rules?: { id: string }[] };
	for (const rule of plan.rules ?? []) {
		await api("DELETE", `/calendar/slots/${rule.id}`).catch(() => undefined);
	}
	await api("PUT", `/calendar/days/${DAY}`, { note: "", theme: "" }).catch(
		() => undefined,
	);
	const list = (await api("GET", "/connections")) as {
		connections: { id: string; kind: string }[];
	};
	for (const connection of list.connections.filter((c) => c.kind === "fake")) {
		await api("DELETE", `/connections/${connection.id}`);
	}
	await network.close();
}

// @chromium-only: the fake network listens on the one port the server of the suite is told about.
test.describe("the day of the publishing calendar @chromium-only", () => {
	test("plan a day: an hour, a post, a note, a theme, a slot, a copy and the file", async ({
		page,
		request,
	}) => {
		test.setTimeout(240_000);
		const title = `Day post ${randomBytes(3).toString("hex")}`;
		const ids: string[] = [];
		const network = await fakeNetwork();
		try {
			await signInAsPerson(page);
			await connectAccount(page);

			const api = await caller(request);
			const made = (await api("POST", "/projects", {
				name: "Day e2e",
				preset: "9:16",
			})) as { project_id: string };
			seedExport(made.project_id);

			// A day opens from the date typed in the header, and from a day of the month.
			await page.goto("/publish");
			await expect(page.getByTestId("calendar-month")).toBeVisible();
			await goToDate(page, DAY);
			await expect(page.getByTestId("publish-mode-day")).toHaveAttribute(
				"aria-pressed",
				"true",
			);
			await expect(page.getByTestId("day-hour")).toHaveCount(24);
			await page.getByTestId("publish-mode-month").click();
			const cell = page.locator(
				`[data-testid="calendar-day"][data-day="${DAY}"]`,
			);
			await cell.getByTestId("calendar-day-open").click();
			await expect(page.getByTestId("calendar-day-view")).toHaveAttribute(
				"data-date",
				DAY,
			);

			// A click on an empty hour opens the editor with that date and time filled in.
			await page.getByTestId("day-hour-add-9").click();
			const dialog = page.getByTestId("publication-editor");
			const panel = dialog.getByTestId("target-panel");
			await expect(panel.getByLabel("Date and time")).toHaveValue(
				`${DAY}T09:00`,
			);
			await dialog.getByLabel("Project").selectOption(made.project_id);
			await dialog.getByLabel("Exported video").selectOption({ index: 1 });
			await dialog.getByLabel("Title", { exact: true }).fill(title);
			await panel.getByLabel("No", { exact: true }).check();
			await dialog.getByRole("button", { name: "Save", exact: true }).click();
			await expect(panel.getByTestId("target-state")).toHaveText("Draft");
			await page.keyboard.press("Escape");
			await expect(dialog).toHaveCount(0);

			const list = (await api(
				"GET",
				`/publications?project_id=${made.project_id}`,
			)) as unknown as { publications: Publication[] };
			const first = list.publications.find((p) => p.title === title);
			expect(first).toBeDefined();
			const id = first?.id ?? "";
			ids.push(id);
			const targetId = first?.targets[0]?.id ?? "";
			const chip = page.locator(
				`[data-testid="target-chip"][data-target-id="${targetId}"]`,
			);

			// The post is in its hour, and nothing was sent.
			await expect(hour(page, 9).locator(chip)).toHaveCount(1);
			expect(network.uploads()).toBe(0);

			// Adjusted in place: its time. The day shows what the service answered, in the new hour.
			await hour(page, 9).getByTestId("day-adjust").click();
			await page.getByLabel("Time", { exact: true }).fill("10:30");
			await page.getByTestId("day-adjust-save").click();
			await expect(hour(page, 10).locator(chip)).toContainText("10:30");
			await expect(hour(page, 9).locator(chip)).toHaveCount(0);

			// Adjusted in place: its text.
			await hour(page, 10).getByTestId("day-adjust").click();
			await page
				.getByLabel("Text for this account")
				.fill("A text of this account");
			await page.getByTestId("day-adjust-save").click();
			await expect
				.poll(
					async () =>
						(await publicationOf(api, id)).targets[0]?.fields.description,
				)
				.toBe("A text of this account");

			// The keyboard moves a post by an hour, as in the week: Space, the arrow, Enter.
			await chip.focus();
			await page.keyboard.press("Space");
			await expect(chip).toHaveAttribute("aria-pressed", "true");
			await page.keyboard.press("ArrowDown");
			await page.keyboard.press("Enter");
			await expect(hour(page, 11).locator(chip)).toContainText("11:30");
			await expect
				.poll(async () => (await publicationOf(api, id)).targets[0]?.state)
				.toBe("waiting_approval");
			expect(network.uploads()).toBe(0);

			// A note and a theme are kept by the service: they are there after a reload.
			await page.getByTestId("day-note").fill("long video day");
			await page.getByTestId("day-note").blur();
			// The choice is the service's: the radio is on once the service answered.
			await page.getByTestId("day-theme-amber").click();
			await expect(page.getByTestId("day-theme-amber")).toBeChecked();
			await expect(page.getByTestId("day-header")).toHaveAttribute(
				"data-theme",
				"amber",
			);
			await page.reload();
			await goToDate(page, DAY);
			await expect(page.getByTestId("day-note")).toHaveValue("long video day");
			await expect(page.getByTestId("day-header")).toHaveAttribute(
				"data-theme",
				"amber",
			);

			// The month shows the theme of the day, and the small month says it has a note.
			await page.getByTestId("publish-mode-month").click();
			await expect(cell).toHaveAttribute("data-theme", "amber");
			await expect(
				page.locator(`[data-testid="mini-day"][data-day="${DAY}"]`),
			).toHaveAttribute("aria-label", /has a note/);

			// A weekly slot, "every Tuesday at 14:00 on the fake network", is an empty slot to fill.
			await page.getByTestId("publish-slots").click();
			const slots = page.getByTestId("slots-dialog");
			await slots.getByLabel("Weekday").selectOption("tue");
			await slots.getByLabel("Time", { exact: true }).fill("14:00");
			await slots.getByTestId("slot-add").click();
			await expect(slots.getByTestId("slot-rule")).toHaveCount(1);
			await page.keyboard.press("Escape");
			await expect(slots).toHaveCount(0);
			await goToDate(page, DAY);
			await expect(hour(page, 14).getByTestId("day-slot-empty")).toHaveCount(1);
			// Nothing was sent for it, and it holds no post.
			expect(network.uploads()).toBe(0);
			await hour(page, 14).getByTestId("day-slot-empty").click();
			await expect(
				page.getByTestId("target-panel").getByLabel("Date and time"),
			).toHaveValue(`${DAY}T14:00`);
			await page.keyboard.press("Escape");

			// The same slot the next week, still at 14:00.
			await goToDate(page, NEXT);
			await expect(hour(page, 14).getByTestId("day-slot-empty")).toHaveCount(1);

			// A copy of the post goes to another day: a new draft, and the original is as it was.
			await goToDate(page, DAY);
			await chip.click();
			const editor = page.getByTestId("publication-editor");
			await expect(editor.getByTestId("target-state")).toHaveText(
				"Waiting for approval",
			);
			await editor.getByTestId("duplicate-at").fill(`${NEXT}T09:00`);
			await editor.getByTestId("duplicate-go").click();
			// The page goes to the day of the copy and opens it.
			await expect(page.getByTestId("calendar-day-view")).toHaveAttribute(
				"data-date",
				NEXT,
			);
			await expect(
				page
					.getByTestId("publication-editor")
					.getByTestId("target-panel")
					.getByLabel("Date and time"),
			).toHaveValue(`${NEXT}T09:00`);
			await expect(
				page.getByTestId("publication-editor").getByTestId("target-state"),
			).toHaveText("Draft");
			await page.keyboard.press("Escape");
			const all = (await api(
				"GET",
				`/publications?project_id=${made.project_id}`,
			)) as unknown as { publications: Publication[] };
			expect(all.publications).toHaveLength(2);
			const copy = all.publications.find((p) => p.id !== id);
			ids.push(copy?.id ?? "");
			expect(copy?.targets[0]?.state).toBe("draft");
			const original = await publicationOf(api, id);
			expect(original.targets[0]?.state).toBe("waiting_approval");
			expect(original.targets[0]?.id).toBe(targetId);

			// The plan as an .ics file: the link of the range, read only, UTC times, no credential or id.
			await page.getByTestId("publish-mode-month").click();
			const href = await page
				.getByTestId("publish-export")
				.getAttribute("href");
			expect(href).toContain("/api/v1/calendar/export?");
			const file = await page.request.get(href ?? "");
			expect(file.status()).toBe(200);
			expect(file.headers()["content-type"]).toContain("text/calendar");
			expect(file.headers()["content-disposition"]).toContain("attachment");
			const body = await file.text();
			expect(body).toContain("BEGIN:VCALENDAR");
			expect(body).toContain(title);
			expect(body.match(/BEGIN:VEVENT/g)).toHaveLength(2);
			expect(body).toMatch(/DTSTART:\d{8}T\d{6}Z/);
			for (const secret of [
				"con_",
				"pub_",
				"tgt_",
				"apr_",
				CLIENT_SECRET,
				"e2e-access",
				"e2e-refresh",
			]) {
				expect(body).not.toContain(secret);
			}
			expect(network.uploads()).toBe(0);
		} finally {
			await cleanUp(request, network, ids);
		}
	});

	test("a quiet day is a rule: an agent is refused, and a person confirms", async ({
		page,
		request,
	}) => {
		test.setTimeout(240_000);
		const ids: string[] = [];
		const network = await fakeNetwork();
		try {
			await signInAsPerson(page);
			await connectAccount(page);

			const api = await caller(request);
			const made = (await api("POST", "/projects", {
				name: "Quiet day e2e",
				preset: "9:16",
			})) as { project_id: string };
			const exportJob = seedExport(made.project_id);
			const accounts = (await api("GET", "/social/accounts")) as {
				accounts: { connection_id: string }[];
			};
			const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
			const created = (await api("POST", "/publications", {
				project_id: made.project_id,
				title: "Quiet day post",
				export: exportJob,
				targets: [
					{
						connection_id: accounts.accounts[0]?.connection_id,
						at: `${NEXT}T16:00`,
						zone,
						fields: { made_for_kids: false },
					},
				],
			})) as unknown as Publication;
			ids.push(created.id);
			const targetId = created.targets[0]?.id ?? "";

			await page.goto("/publish");
			await goToDate(page, NEXT);

			// A person marks the day, and the day says so in words.
			await page.getByTestId("day-quiet").click();
			await expect(page.getByTestId("day-quiet")).toBeChecked();
			await expect(page.getByTestId("day-quiet-badge")).toContainText(
				"No posts",
			);

			// An agent (the local token) that schedules on it is refused, and nothing changes.
			await expect(
				api("POST", `/publications/${created.id}/schedule`, {
					target_id: targetId,
				}),
			).rejects.toThrow(/E_PRIVILEGED_SESSION/);
			await expect(
				api("PUT", "/calendar/quiet/2027-03-16", { zone, quiet: true }),
			).rejects.toThrow(/E_PRIVILEGED_SESSION/);
			expect((await publicationOf(api, created.id)).targets[0]?.state).toBe(
				"draft",
			);

			// The person is asked first; a no schedules nothing, a yes does.
			await page
				.locator(`[data-testid="target-chip"][data-target-id="${targetId}"]`)
				.click();
			const editor = page.getByTestId("publication-editor");
			await editor.getByTestId("target-schedule").click();
			const ask = page.getByTestId("quiet-confirm");
			await expect(ask).toBeVisible();
			await ask.getByTestId("quiet-confirm-no").click();
			await expect(ask).toHaveCount(0);
			await expect(editor.getByTestId("target-state")).toHaveText("Draft");
			expect((await publicationOf(api, created.id)).targets[0]?.state).toBe(
				"draft",
			);

			await editor.getByTestId("target-schedule").click();
			await page.getByTestId("quiet-confirm-yes").click();
			await expect(editor.getByTestId("target-state")).toHaveText(
				"Waiting for approval",
			);
			expect(network.uploads()).toBe(0);
			await page.keyboard.press("Escape");

			// The person clears the mark.
			await page.getByTestId("day-quiet").click();
			await expect(page.getByTestId("day-quiet")).not.toBeChecked();
			await expect(page.getByTestId("day-quiet-badge")).toHaveCount(0);
		} finally {
			await cleanUp(request, network, ids);
		}
	});
});
