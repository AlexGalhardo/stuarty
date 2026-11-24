import { randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import { caller, expect, projectBody, test } from "./helpers";

// Epic 80 (D-134, D-137): a text-only project is a complete project. A copy is written, reviewed and ended in Copy
// and Export with no destination; a blog is a separate choice. The words and the reading time come from the
// service; the agent panel of a text project offers text actions and says what does not apply. Nothing here
// assumes a video.
test.describe.configure({ mode: "serial" });

function words(n: number): string {
	return Array.from({ length: n }, (_, i) => `word${i}`).join(" ");
}

test.describe("write text flow", () => {
	test("a copy: words and reading time from the service, then Copy and Export with no destination", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(browserName !== "chromium", "the clipboard is read in Chromium"); // verify:env-skip the clipboard permission is granted in Chromium only
		await page
			.context()
			.grantPermissions(["clipboard-read", "clipboard-write"]);
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const made = await api("POST", "/projects", {
			...projectBody(`Copy ${tag}`),
			kind: "text",
		});
		const projectId = made.project_id as string;
		const created = (await api("POST", `/projects/${projectId}/texts`, {
			title: `Launch copy ${tag}`,
			text_kind: "copy",
			text: words(401),
		})) as { item: { id: string } };
		const itemId = created.item.id;

		await page.goto("/write");
		await page.locator(`[data-text-id="${itemId}"]`).click();
		const meta = page.getByTestId("text-meta");
		await expect(meta).toContainText("401 words");
		// The service says 401 words take 3 minutes: the page does not do that sum.
		await expect(meta).toContainText("3 min read");

		const editor = page.getByTestId("text-editor");
		await editor.getByLabel("Text (Markdown)").fill("Short and sweet today.");
		await editor.getByRole("button", { name: "Save", exact: true }).click();
		await expect(page.getByTestId("save-status")).toHaveText(
			"Saved as revision 2.",
		);
		const stored = (await api(
			"GET",
			`/projects/${projectId}/texts/${itemId}`,
		)) as { item: { words: number } };
		expect(stored.item.words).toBe(4);
		await expect(meta).toContainText("4 words");
		await expect(meta).toContainText("1 min read");

		// Save, then Review, then the end of the copy: one primary action at a time.
		const end = page.getByTestId("end-panel");
		await expect(
			editor.getByRole("button", { name: "Review", exact: true }),
		).toHaveAttribute("data-primary", "true");
		await expect(end.locator('[data-primary="true"]')).toHaveCount(0);
		await editor.getByRole("button", { name: "Review", exact: true }).click();
		await expect(end.getByRole("button", { name: "Copy" })).toHaveAttribute(
			"data-primary",
			"true",
		);
		await expect(page.locator('[data-primary="true"]')).toHaveCount(1);

		// No destination and no connection is asked for.
		await expect(end).toContainText("No destination is needed");
		await expect(end.getByRole("link", { name: "Open Publish" })).toHaveCount(
			0,
		);
		await end.getByRole("button", { name: "Copy" }).click();
		await expect(page.getByTestId("end-status")).toContainText(
			"The saved text is on your clipboard.",
		);
		expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
			"Short and sweet today.",
		);

		const download = page.waitForEvent("download");
		await end.getByRole("button", { name: "Export" }).click();
		const file = await download;
		expect(file.suggestedFilename()).toBe(`Launch-copy-${tag}.md`);
		const path = await file.path();
		expect(await readFile(path, "utf8")).toBe("Short and sweet today.");

		// Sending it to a blog is a choice: it starts closed, and opening it sends nothing.
		const blog = end.getByRole("button", { name: "Send it to a blog" });
		await expect(blog).toHaveAttribute("aria-expanded", "false");
		await blog.click();
		await expect(page.getByTestId("end-blog")).toContainText(
			"Nothing is sent from here",
		);
		const publications = (await api(
			"GET",
			`/projects/${projectId}/describe`,
		)) as { items: { kind: string }[] };
		expect(publications.items.filter((i) => i.kind === "publication")).toEqual(
			[],
		);
	});

	test("a text project's agent panel offers text actions and no video one, a video project's does not say it", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const text = await api("POST", "/projects", {
			...projectBody(`Text ${tag}`),
			kind: "text",
		});
		const textId = text.project_id as string;
		const copy = (await api("POST", `/projects/${textId}/texts`, {
			title: `Only text ${tag}`,
			text_kind: "copy",
			text: "Words only.",
		})) as { item: { id: string } };

		await page.goto("/write");
		await page.locator(`[data-text-id="${copy.item.id}"]`).click();
		const agent = page.getByTestId("agent-panel");
		await expect(agent.getByTestId("agent-text-only")).toContainText(
			"Cut, video caption and export do not apply.",
		);
		// The skill, whether it is loaded and the tool are in view even before a session exists.
		await expect(agent.getByTestId("chat-activity-skill")).toBeVisible();
		await expect(agent.getByTestId("chat-activity-tool")).toBeVisible();
		await expect(agent).toContainText("No agent session has started");
		// The path of the project names no video step.
		const path = page.getByTestId("flow-path");
		await expect(path).toBeVisible();
		for (const word of ["Record", "Edit", "Finish"]) {
			await expect(path).not.toContainText(word);
		}
		await expect(path).toContainText("Review");

		const video = await api("POST", "/projects", {
			...projectBody(`Video ${tag}`),
			kind: "video",
		});
		const videoId = video.project_id as string;
		const script = (await api("POST", `/projects/${videoId}/texts`, {
			title: `Script ${tag}`,
			text_kind: "script",
			text: "VO: hello.",
		})) as { item: { id: string } };
		await page.goto("/write");
		await page.locator(`[data-text-id="${script.item.id}"]`).click();
		await expect(page.getByTestId("agent-panel")).toBeVisible();
		await expect(page.getByTestId("agent-text-only")).toHaveCount(0);
	});

	test("a post goes on to the next step of the path, an explicit choice and never a default", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const made = await api("POST", "/projects", {
			...projectBody(`Blog ${tag}`),
			kind: "text",
		});
		const projectId = made.project_id as string;
		const created = (await api("POST", `/projects/${projectId}/texts`, {
			title: `Post ${tag}`,
			text_kind: "post",
			text: `---\ntitle: Post ${tag}\nslug: post-${tag}\nsummary: A line.\nlanguage: en\n---\n# Body\n\nWords.\n`,
		})) as { item: { id: string } };
		await page.goto("/write");
		await page.locator(`[data-text-id="${created.item.id}"]`).click();
		await page
			.getByTestId("text-editor")
			.getByRole("button", { name: "Review", exact: true })
			.click();
		const next = page
			.getByTestId("end-panel")
			.getByRole("link", { name: /^Next: / });
		await expect(next).toHaveAttribute("data-primary", "true");
		await expect(next).toHaveAttribute("href", /^\/(approvals|publish)$/);
		await expect(page.locator('[data-primary="true"]')).toHaveCount(1);
		// A post carries no blog choice of its own: its path is the publication.
		await expect(
			page.getByRole("button", { name: "Send it to a blog" }),
		).toHaveCount(0);
	});
});
