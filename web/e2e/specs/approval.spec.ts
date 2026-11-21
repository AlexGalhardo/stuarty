import { randomBytes, randomInt } from "node:crypto";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { caller, expect, home, test } from "./helpers";

// ADR-0032, epic 05 (SDD-015): the queue shows what waits for a person with where it goes, what it
// costs and who asked, and updates without a reload; a person approves or rejects; a rule is written
// here; a ceiling is set here; and the local token an agent holds cannot do any of it.
//
// A request is made by a job kind that needs approval, and the product registers none until the
// epics of publishing and generation, so the spec writes the rows of the requests into the database
// of the workspace the way that code will, and the real server reads and decides them.
test.describe.configure({ mode: "serial" });

const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

function newId(prefix: string): string {
	let out = "";
	for (let i = 0; i < 10; i++) out += ALPHABET[randomInt(ALPHABET.length)];
	return `${prefix}_${out}`;
}

interface Seed {
	summary: string;
	actor?: string;
	connection?: string;
	micros?: number;
}

function seed(items: Seed[]): string[] {
	const db = new DatabaseSync(join(home, "db.sqlite"));
	try {
		db.exec("PRAGMA busy_timeout = 10000");
		const insert = db.prepare(
			`INSERT INTO approval_requests
 (id, actor, kind, summary, connection_id, payload_ref, estimate_micros, currency, state, created_ms, expires_ms)
 VALUES (?, ?, 'publish.post', ?, ?, 'draft:e2e', ?, ?, 'pending', ?, ?)`,
		);
		const now = Date.now();
		return items.map((item, i) => {
			const id = newId("apr");
			insert.run(
				id,
				item.actor ?? "local:claude-code",
				item.summary,
				item.connection ?? null,
				item.micros ?? 0,
				item.micros ? "USD" : "",
				now + i,
				now + 7 * 24 * 3600 * 1000,
			);
			return id;
		});
	} finally {
		db.close();
	}
}

const unique = (browserName: string) =>
	`${browserName}-${randomBytes(3).toString("hex")}`;

async function connect(
	api: Awaited<ReturnType<typeof caller>>,
	label: string,
): Promise<string> {
	const made = await api("POST", "/connections", {
		kind: "test",
		label,
		endpoint: "http://127.0.0.1:9/health",
		accepts: ["text"],
		secret: "sk-e2e-approval-CANARY",
	});
	return String(made.id);
}

// A person decides in the desktop app, whose launch code carries the launcher secret (SDD-007
// section 3.4 item 7); the server of this suite is started with the secret of LAUNCHER_SECRET.
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

async function openQueue(page: Page): Promise<void> {
	await signInAsPerson(page);
	await page.goto("/approvals");
	await expect(
		page.getByRole("heading", { name: "Approvals", level: 1 }),
	).toBeVisible();
	await expect(page.getByTestId("approval-live")).toHaveAttribute(
		"data-live",
		"true",
	);
}

test.describe("approval queue", () => {
	test("shows the summary, the destination, the cost and the actor, and approves", async ({
		page,
		request,
		browserName,
	}) => {
		const tag = unique(browserName);
		const api = await caller(request);
		const connection = await connect(api, `Blog ${tag}`);
		// The connection is removed at the end: the specs share one workspace, and a
		// connection left behind shows on the pages of the others (VV-253).
		try {
			const [id] = seed([
				{
					summary: `Post the launch video ${tag}`,
					actor: "key:key_e2e",
					connection,
					micros: 2_500_000,
				},
			]);

			await openQueue(page);
			const row = page.locator(`[data-approval-id="${id}"]`);
			await expect(row.getByTestId("approval-summary")).toHaveText(
				`Post the launch video ${tag}`,
			);
			await expect(row.getByTestId("approval-destination")).toHaveText(
				`Blog ${tag}`,
			);
			await expect(row.getByTestId("approval-cost")).toHaveText("$2.50");
			await expect(row.getByTestId("approval-actor")).toHaveText("key:key_e2e");

			// A person's session (the desktop app) is offered the answers: enabled, and no note (VV-319).
			await expect(row.getByRole("button", { name: "Approve" })).toBeEnabled();
			await expect(page.locator("#approvals-person-only")).toHaveCount(0);
			await row.getByRole("button", { name: "Approve" }).click();
			await expect(row).toHaveCount(0);

			const decided = await api("GET", `/approvals/${id}`);
			expect(decided).toMatchObject({
				id,
				state: "approved",
				decided_by: "local:ui",
			});
		} finally {
			await api("DELETE", `/connections/${connection}`);
		}
	});

	test("a decision made in another window removes the request here without a reload", async ({
		page,
		context,
		request,
		browserName,
	}) => {
		const tag = unique(browserName);
		const [id] = seed([{ summary: `Live ${tag}` }]);

		await openQueue(page);
		const row = page.locator(`[data-approval-id="${id}"]`);
		await expect(row).toBeVisible();

		// The first page never navigates again: the stream carries the decision to it.
		const other = await context.newPage();
		await openQueue(other);
		const there = other.locator(`[data-approval-id="${id}"]`);
		await there.getByLabel("Reason").fill("wrong channel");
		await there.getByRole("button", { name: "Reject" }).click();
		await expect(there).toHaveCount(0);
		await expect(row).toHaveCount(0);

		const api = await caller(request);
		expect(await api("GET", `/approvals/${id}`)).toMatchObject({
			state: "rejected",
			reason: "wrong channel",
		});
	});

	test("text an agent wrote is shown as text", async ({
		page,
		browserName,
	}) => {
		const tag = unique(browserName);
		const payload = `<img src=x onerror=window.__pwned=1> ${tag}`;
		const [id] = seed([{ summary: payload, actor: payload }]);

		await openQueue(page);
		const row = page.locator(`[data-approval-id="${id}"]`);
		await expect(row.getByTestId("approval-summary")).toHaveText(payload);
		expect(await page.evaluate(() => "__pwned" in window)).toBe(false);
		await expect(row.locator("img")).toHaveCount(0);
	});

	test("the local token an agent holds can read the queue and cannot decide or write a rule", async ({
		request,
		browserName,
	}) => {
		const tag = unique(browserName);
		const [id] = seed([{ summary: `Agent ${tag}` }]);
		const agent = await caller(request, "claude-code");

		expect(await agent("GET", `/approvals/${id}`)).toMatchObject({
			state: "pending",
		});

		for (const [method, path, body] of [
			["POST", `/approvals/${id}/decide`, { decision: "approve" }],
			["POST", "/approval-rules", { kind: "publish.post" }],
			["PUT", "/cost/ceilings", { currency: "USD", per_day_micros: 1 }],
		] as const) {
			let failure = "";
			try {
				await agent(method, path, body);
			} catch (error) {
				failure = String(error);
			}
			expect(failure, `${method} ${path}`).toContain("403");
			expect(failure, `${method} ${path}`).toContain("E_PRIVILEGED_SESSION");
		}

		expect(await agent("GET", `/approvals/${id}`)).toMatchObject({
			state: "pending",
		});
	});

	test("a person writes a rule and a ceiling in the app", async ({
		page,
		request,
	}) => {
		// A rule names a kind an action has (VV-307); the project id, which need not exist, tells
		// this run's rule from the rule of another browser.
		const alphabet = "0123456789abcdefghjkmnpqrstvwxyz";
		const project = `prj_${Array.from(randomBytes(10), (b) => alphabet[b % 32]).join("")}`;
		await openQueue(page);

		// A kind no action has would never apply, so the form shows the refusal and its hint.
		await page.getByLabel("Kind of action").fill("publish.post");
		await page.getByRole("button", { name: "Add rule" }).click();
		await expect(page.getByText("no action has this kind")).toBeVisible();
		await expect(
			page.getByText("publish.scheduled, publish.upload"),
		).toBeVisible();

		// The example of the label is a kind an action has.
		await page
			.getByLabel("Kind of action (for example publish.upload)")
			.fill("text.generate");
		// The rule form comes before the ceiling form, which has a project field too.
		await page.getByLabel("Project id").first().fill(project);
		await page.getByLabel("Most per item").first().fill("2.5");
		await page.getByRole("button", { name: "Add rule" }).click();

		const rule = page.locator("[data-rule-id]").filter({ hasText: project });
		await expect(rule).toContainText("text.generate");
		await expect(rule).toContainText("Most per item: $2.50");

		const api = await caller(request);
		const listed = (await api("GET", "/approval-rules")) as {
			rules: {
				id: string;
				kind: string;
				project_id: string | null;
				per_item_micros: number | null;
			}[];
		};
		const stored = listed.rules.find((r) => r.project_id === project);
		expect(stored?.kind).toBe("text.generate");
		expect(stored?.per_item_micros).toBe(2_500_000);

		await rule.getByRole("button", { name: "Remove" }).click();
		await expect(rule).toHaveCount(0);

		await page.getByLabel("Most per day").last().fill("9");
		await page.getByRole("button", { name: "Save ceiling" }).click();
		await expect(page.getByTestId("cost-ceilings")).toContainText("$9.00");
		const cost = (await api("GET", "/cost")) as {
			ceilings: { per_day_micros: number | null }[];
		};
		expect(cost.ceilings.some((c) => c.per_day_micros === 9_000_000)).toBe(
			true,
		);

		// Leave the workspace with no ceiling: both limits empty remove it.
		await page.getByRole("button", { name: "Save ceiling" }).click();
		await expect(page.getByTestId("cost-ceilings")).toHaveCount(0);
	});

	test("the page is in Portuguese when the language is", async ({
		page,
		browserName,
	}) => {
		const tag = unique(browserName);
		const [id] = seed([{ summary: `Em português ${tag}`, micros: 1_000_000 }]);
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await signInAsPerson(page);
		await page.goto("/approvals");
		await expect(
			page.getByRole("heading", { name: "Aprovações", level: 1 }),
		).toBeVisible();
		const row = page.locator(`[data-approval-id="${id}"]`);
		await expect(row).toContainText("Aprovar");
		await expect(row).toContainText("Rejeitar");
		await expect(row).toContainText("Custo estimado");
		await expect(page.getByTestId("approval-live")).toContainText("Ao vivo");
		await row.getByRole("button", { name: "Rejeitar" }).click();
		await expect(row).toHaveCount(0);
	});
});
