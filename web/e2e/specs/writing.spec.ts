import { randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { join } from "node:path";
import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { caller, expect, home, test } from "./helpers";

// Epic 17 (SDD-019, RF-350 to RF-354): a creator writes the script of a video, sees how long it takes, writes the
// caption of each network and sees its counters, asks a model for a draft that waits for approval and lands as a
// new revision, and goes back to an older revision. The counts, the estimates and the problems are the core's; the
// provider is a server of this spec on loopback, so no real provider is called.
test.describe.configure({ mode: "serial" });

const KEY = "sk-e2e-CANARY-17b2d9";
const PAYLOAD = "<img src=x onerror=window.__pwned=1>";
const DRAFT = "Sourdough, from starter to slice.";

interface FakeProvider {
	url: string;
	chats: number;
	prompts: string[];
	close(): Promise<void>;
}

async function fakeProvider(model: string): Promise<FakeProvider> {
	const fake: FakeProvider = {
		url: "",
		chats: 0,
		prompts: [],
		close: () => Promise.resolve(),
	};
	const server: Server = createServer((request, response) => {
		response.setHeader("content-type", "application/json");
		if (request.url === "/v1/models") {
			response.end(JSON.stringify({ object: "list", data: [{ id: model }] }));
			return;
		}
		if (request.url === "/v1/chat/completions") {
			fake.chats += 1;
			const chunks: Buffer[] = [];
			request.on("data", (chunk: Buffer) => chunks.push(chunk));
			request.on("end", () => {
				const sent = JSON.parse(Buffer.concat(chunks).toString("utf8")) as {
					messages: { content: string }[];
				};
				fake.prompts.push(sent.messages.at(-1)?.content ?? "");
				response.end(
					JSON.stringify({
						model,
						choices: [{ message: { content: DRAFT }, finish_reason: "stop" }],
						usage: { prompt_tokens: 10, completion_tokens: 20 },
					}),
				);
			});
			return;
		}
		response.statusCode = 404;
		response.end("{}");
	});
	await new Promise<void>((resolve) =>
		server.listen(0, "127.0.0.1", () => resolve()),
	);
	const { port } = server.address() as AddressInfo;
	fake.url = `http://127.0.0.1:${port}/v1`;
	fake.close = () => new Promise((resolve) => server.close(() => resolve()));
	return fake;
}

// A person decides in the desktop app, whose launch code carries the launcher secret (SDD-007 section 3.4 item 7).
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

function words(n: number): string {
	return Array.from({ length: n }, () => "word").join(" ");
}

test.describe("writing", () => {
	test("a script: write, see the duration at two paces, save, read the preview as text, go back", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const made = await api("POST", "/projects", { name: `Bread ${tag}` });
		const projectId = made.project_id as string;

		await page.goto("/write");
		await expect(
			page.getByRole("heading", { name: "Write", level: 1 }),
		).toBeVisible();

		await page.getByRole("button", { name: "New text" }).click();
		const form = page.getByRole("form", { name: "New text" });
		await form.getByLabel("Project").selectOption(projectId);
		await form.getByLabel("Title").fill(`Hook ${tag}`);
		await form.getByLabel("Kind").selectOption("script");
		await form.getByRole("button", { name: "Create" }).click();

		const editor = page.getByTestId("text-editor");
		await expect(editor).toBeVisible();

		// 75 words of narration are 30 seconds at the default 150 words per minute, 15 at 300.
		const body = `# Hook\nVO: ${words(75)}\n\nON SCREEN: ${PAYLOAD}`;
		await editor.getByLabel("Text (Markdown)").fill(body);
		await expect(page.getByTestId("script-duration")).toContainText("0:30.0");
		await page.getByLabel("Words per minute").fill("300");
		await expect(page.getByTestId("script-duration")).toContainText("0:15.0");
		await expect(page.getByTestId("script-panel")).toContainText(
			"75 words spoken",
		);

		await editor.getByRole("button", { name: "Save", exact: true }).click();
		await expect(page.getByTestId("save-status")).toHaveText(
			"Saved as revision 2.",
		);

		// What the service stored is what was typed, and the revision says who.
		const list = (await api("GET", `/texts?project_id=${projectId}`)) as {
			texts: {
				id: string;
				revision: number;
				text_kind: string;
				words: number;
			}[];
		};
		expect(list.texts).toHaveLength(1);
		expect(list.texts[0]).toMatchObject({ revision: 2, text_kind: "script" });
		const itemId = list.texts[0]?.id as string;

		// The preview draws text nodes only: the markup the script holds is shown, never built.
		await editor.getByRole("tab", { name: "Preview" }).click();
		const preview = page.getByTestId("write-preview");
		await expect(preview).toContainText(PAYLOAD);
		await expect(preview.locator("img")).toHaveCount(0);
		expect(
			await page.evaluate(() => (window as { __pwned?: number }).__pwned),
		).toBeUndefined();

		// A second save and the way back: the older revision comes back as a new one.
		await editor.getByRole("tab", { name: "Write" }).click();
		await editor.getByLabel("Text (Markdown)").fill("# Hook\nVO: another take");
		await editor.getByRole("button", { name: "Save", exact: true }).click();
		await expect(page.getByTestId("save-status")).toHaveText(
			"Saved as revision 3.",
		);

		const revisions = page.getByTestId("revisions-panel");
		await expect(revisions.locator("[data-revision]")).toHaveCount(3);
		await revisions
			.locator('[data-revision="2"]')
			.getByRole("button", { name: "Compare with the one before" })
			.click();
		await expect(page.getByTestId("revision-diff")).toContainText(
			"Changes from revision 1 to 2",
		);

		await revisions
			.locator('[data-revision="2"]')
			.getByRole("button", { name: "Go back to this revision" })
			.click();
		await expect(page.getByTestId("save-status")).toHaveText(
			"Went back to revision 2: it is revision 4 now.",
		);
		await expect(editor.getByLabel("Text (Markdown)")).toHaveValue(body);

		const reverted = (await api(
			"GET",
			`/projects/${projectId}/texts/${itemId}`,
		)) as {
			body: string;
			revision: { revision: number; source: string; reverted_to: number };
		};
		expect(reverted.body).toBe(body);
		expect(reverted.revision).toMatchObject({
			revision: 4,
			source: "revert",
			reverted_to: 2,
		});
		// Revisions 1 to 3 are all still there.
		const all = (await api(
			"GET",
			`/projects/${projectId}/texts/${itemId}/revisions`,
		)) as {
			revisions: { revision: number }[];
		};
		expect(all.revisions.map((r) => r.revision)).toEqual([4, 3, 2, 1]);

		// The overview of the project lists the text, and the description says its kind.
		const described = (await api("GET", `/projects/${projectId}/describe`)) as {
			text: string;
			production: { texts: number };
		};
		expect(described.production.texts).toBe(1);
		expect(described.text).toContain("text_kind=script");

		const violations = (
			await new AxeBuilder({ page }).include("main").analyze()
		).violations.filter(
			(v) => v.impact === "serious" || v.impact === "critical",
		);
		expect(violations.map((v) => v.id)).toEqual([]);
	});

	test("a caption: counters per network, the exact limit broken, and a conflict that keeps the text", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const made = await api("POST", "/projects", { name: `Caption ${tag}` });
		const projectId = made.project_id as string;
		const created = (await api("POST", `/projects/${projectId}/texts`, {
			title: `Caption ${tag}`,
			text_kind: "caption",
			text: "Fresh bread #sourdough\n\n## instagram\nHello #bread",
		})) as { item: { id: string } };

		await page.goto("/write");
		await page.locator(`[data-text-id="${created.item.id}"]`).click();

		const panel = page.getByTestId("caption-panel");
		const instagram = panel.locator('[data-network="instagram"]');
		await expect(instagram).toContainText("Instagram");
		await expect(instagram.getByTestId("caption-count")).toContainText(
			"12 of 2200 characters",
		);
		await expect(instagram.getByTestId("caption-count")).toContainText(
			"hashtags: 1",
		);
		await expect(panel.locator('[data-network="common"]')).toContainText(
			"Common text",
		);

		// 2,201 characters break the limit of Instagram by one, and the core says so, with the figures.
		const editor = page.getByTestId("text-editor");
		await editor
			.getByLabel("Text (Markdown)")
			.fill(`## instagram\n${"a".repeat(2201)}\n\n## x\n${"b".repeat(281)}`);
		await expect(instagram).toContainText("2201 of 2200 characters");
		await expect(instagram).toContainText(
			"allows 2200 characters and the text has 2201",
		);
		await expect(panel.locator('[data-network="x"]')).toContainText(
			"allows 280 weighted characters and the text has 281",
		);

		// Someone else saves while this editor is open: the write is refused and the typed text stays.
		await api("PUT", `/projects/${projectId}/texts/${created.item.id}`, {
			text: "written by an agent",
		});
		await editor.getByRole("button", { name: "Save", exact: true }).click();
		await expect(
			page.getByRole("alert").filter({ hasText: "Someone saved revision 2" }),
		).toBeVisible();
		await expect(editor.getByLabel("Text (Markdown)")).toHaveValue(
			/^## instagram\na{2201}/,
		);
		await page
			.getByRole("button", { name: "Read the latest revision" })
			.click();
		await expect(editor.getByLabel("Text (Markdown)")).toHaveValue(
			"written by an agent",
		);
	});

	test("a draft with a model: the estimate first, the approval, then a new revision with its cost", async ({
		page,
		request,
		browserName,
	}) => {
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const modelId = `gpt-${tag}`;
		const fake = await fakeProvider(modelId);
		const label = `OpenAI ${tag}`;
		const agent = await caller(request);
		const made = await agent("POST", "/projects", { name: `Draft ${tag}` });
		const projectId = made.project_id as string;
		await agent("PUT", `/projects/${projectId}/brief`, {
			text: "A short video about sourdough.",
		});
		const created = (await agent("POST", `/projects/${projectId}/texts`, {
			title: `Caption ${tag}`,
			text_kind: "caption",
			text: "## instagram\nold words",
		})) as { item: { id: string } };

		try {
			await signInAsPerson(page);
			await page.goto("/write");
			const priced = await page.evaluate(async (model) => {
				const response = await fetch("/api/v1/providers/prices", {
					method: "PUT",
					credentials: "same-origin",
					headers: {
						Accept: "application/json",
						"Content-Type": "application/json",
					},
					body: JSON.stringify({
						provider: "openai",
						model,
						currency: "USD",
						input_micros_per_mtok: 1_000_000,
						output_micros_per_mtok: 2_000_000,
					}),
				});
				return response.status;
			}, modelId);
			expect(priced).toBe(200);
			await agent("POST", "/connections", {
				kind: "openai",
				label,
				endpoint: fake.url,
				accepts: ["text"],
				secret: KEY,
			});

			await page.goto("/write");
			await page.locator(`[data-text-id="${created.item.id}"]`).click();
			const draft = page.getByTestId("draft-panel");
			await draft.getByLabel("Connection").selectOption({ label });
			await expect(draft.getByLabel("Model")).toHaveValue(modelId);
			await draft.getByLabel("Network").selectOption("instagram");
			await draft.getByLabel("What to write").fill("Make it punchy");

			await draft.getByRole("button", { name: "Estimate the cost" }).click();
			await expect(draft.getByTestId("draft-estimate")).toContainText(
				"Estimated cost:",
			);
			expect(fake.chats).toBe(0);

			await draft.getByRole("button", { name: "Ask for the draft" }).click();
			await expect(draft.getByTestId("draft-waiting")).toContainText(
				"Waiting for your approval.",
			);
			expect(fake.chats).toBe(0);
			const before = (await agent(
				"GET",
				`/projects/${projectId}/texts/${created.item.id}/revisions`,
			)) as {
				revisions: unknown[];
			};
			expect(before.revisions).toHaveLength(1);

			// A person approves in the queue.
			await page.goto("/approvals");
			const row = page
				.locator("[data-approval-id]")
				.filter({ hasText: "Make it punchy" });
			await row.getByRole("button", { name: "Approve" }).click();
			await expect(row).toHaveCount(0);

			await expect
				.poll(async () => {
					const r = (await agent(
						"GET",
						`/projects/${projectId}/texts/${created.item.id}/revisions`,
					)) as { revisions: { source: string }[] };
					return r.revisions[0]?.source;
				})
				.toBe("generated");
			expect(fake.chats).toBe(1);
			// The brief and the instruction reached the model; the key did not reach the page.
			expect(fake.prompts[0]).toContain("A short video about sourdough.");
			expect(fake.prompts[0]).toContain("Make it punchy");

			await page.goto("/write");
			await page.locator(`[data-text-id="${created.item.id}"]`).click();
			const revisions = page.getByTestId("revisions-panel");
			const generated = revisions.locator('[data-revision="2"]');
			await expect(generated).toContainText("drafted by a model");
			await expect(generated).toContainText("openai");
			await expect(generated).toContainText(modelId);
			await expect(generated).toContainText("$0.00005");
			await expect(page.locator("body")).not.toContainText(KEY);
			// Only the variant of the network was replaced, and the first revision is intact.
			await expect(
				page.getByTestId("text-editor").getByLabel("Text (Markdown)"),
			).toHaveValue(`## instagram\n${DRAFT}`);
			await generated
				.getByRole("button", { name: "Go back to this revision" })
				.isVisible();
			await revisions
				.locator('[data-revision="1"]')
				.getByRole("button", { name: "Read" })
				.click();
			await expect(page.getByTestId("revision-body")).toContainText(
				"old words",
			);
		} finally {
			await fake.close();
		}
	});
});
