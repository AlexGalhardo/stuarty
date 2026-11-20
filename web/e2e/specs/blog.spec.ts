import { randomBytes } from "node:crypto";
import { existsSync } from "node:fs";
import { mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { LAUNCHER_SECRET } from "../support/launcher";
import { caller, expect, home, test } from "./helpers";

// Epic 23 (SDD-019 section 13, SDD-020 part 5, RF-353, RF-420 to RF-423): a creator writes a blog post, sees the
// check of the core and a preview drawn from text, makes a destination (a folder of files for a static site), plans
// the publication, a person approves it, and the folder appears at the time of the post, once. The service does the
// work: this spec drives the pages and reads what the core stored and wrote.
test.describe.configure({ mode: "serial" });

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

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

function localField(date: Date): string {
	const pad = (n: number): string => String(n).padStart(2, "0");
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

interface Target {
	id: string;
	state: string;
	network: string;
	remote_id?: string;
	error?: { message: string };
}

interface Publication {
	id: string;
	post?: string;
	targets: Target[];
}

test.describe("blog @chromium-only", () => {
	test("write a post, check it, make a destination, plan it, approve it, and the files appear once", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(300_000);
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const slug = `how-i-cut-${tag}`;
		const made = await api("POST", "/projects", { name: `Blog ${tag}` });
		const projectId = made.project_id as string;
		const folder = await mkdtemp(join(tmpdir(), "stuarty-blog-"));

		await signInAsPerson(page);

		// 1. The post: a front matter and a body, saved as a revision of a text of the kind post.
		await page.goto("/write");
		await page.getByRole("button", { name: "New text" }).click();
		const form = page.getByRole("form", { name: "New text" });
		await form.getByLabel("Project").selectOption(projectId);
		await form.getByLabel("Title").fill(`Post ${tag}`);
		await form.getByLabel("Kind").selectOption("post");
		await form.getByRole("button", { name: "Create" }).click();

		const editor = page.getByTestId("text-editor");
		await expect(editor).toBeVisible();
		// No front matter yet: the core says what is missing.
		await expect(page.getByTestId("post-issues")).toContainText("title");

		const body = `---\ntitle: How I cut a video ${tag}\nslug: ${slug}\nsummary: One short sentence.\ntags: video, editing\nlanguage: en\n---\n# Intro\n\nA *short* post. ${PAYLOAD}\n`;
		await editor.getByLabel("Text (Markdown)").fill(body);
		await editor.getByRole("button", { name: "Save", exact: true }).click();
		await expect(page.getByTestId("save-status")).toContainText(
			"Saved as revision 2",
		);

		const panel = page.getByTestId("post-panel");
		await expect(panel).toContainText("Ready to publish");
		await expect(page.getByTestId("post-slug")).toContainText(slug);
		await expect(page.getByTestId("post-issues")).toHaveCount(0);
		// The preview is text nodes: the markup the post holds is shown, never built.
		await expect(panel.getByTestId("write-preview")).toContainText(PAYLOAD);
		await expect(panel.locator("img")).toHaveCount(0);
		expect(
			await page.evaluate(() => (window as { __pwned?: number }).__pwned),
		).toBeUndefined();

		// The service returns the sanitised HTML: the payload is text in it as well.
		const list = (await api("GET", `/texts?project_id=${projectId}`)) as {
			texts: { id: string }[];
		};
		const postId = list.texts[0]?.id as string;
		const checked = (await api(
			"GET",
			`/projects/${projectId}/texts/${postId}/post`,
		)) as { ok: boolean; html: string };
		expect(checked.ok).toBe(true);
		expect(checked.html).toContain(
			"&lt;img src=x onerror=window.__pwned=1&gt;",
		);
		expect(checked.html).not.toContain("<img");

		// A post with a bad slug is refused with a sentence, and nothing is sent.
		await editor
			.getByLabel("Text (Markdown)")
			.fill(body.replace(`slug: ${slug}`, "slug: Bad Slug"));
		await editor.getByRole("button", { name: "Save", exact: true }).click();
		await expect(page.getByTestId("post-issues")).toContainText("slug");
		await editor.getByLabel("Text (Markdown)").fill(body);
		await editor.getByRole("button", { name: "Save", exact: true }).click();
		await expect(page.getByTestId("post-slug")).toContainText(slug);

		// 2. The destination: a folder for a static site, made the way a person does it in Connections.
		await page.goto("/connections");
		const blog = page.getByTestId("blog-section");
		await blog
			.getByLabel("Destination", { exact: true })
			.selectOption("blog_files");
		await blog.getByLabel("Name").fill(`Site ${tag}`);
		await blog.getByLabel("Folder", { exact: true }).fill(folder);
		await blog.getByLabel("Front matter of the files").selectOption("hugo");
		await blog.getByRole("button", { name: "Add destination" }).click();
		await expect(blog.getByRole("status")).toContainText(
			"The destination was added",
		);
		await expect(
			page.locator("[data-connection-id]").filter({ hasText: `Site ${tag}` }),
		).toHaveCount(1);
		// The folder is a setting kept in the vault: no page and no result shows it again.
		const listed = JSON.stringify(await api("GET", "/connections"));
		expect(listed).not.toContain(folder.replaceAll("\\", "\\\\"));

		// 3. The publication: a post to that destination, published, a minute from now.
		await page.goto("/publish");
		await page.getByTestId("publish-new").click();
		const dialog = page.getByTestId("publication-editor");
		await dialog.getByLabel("Project").selectOption(projectId);
		await dialog.getByLabel("What you publish").selectOption("post");
		await dialog.getByLabel("Blog post", { exact: true }).selectOption({
			label: `Post ${tag}`,
		});
		await dialog
			.getByLabel("Title", { exact: true })
			.fill(`How I cut a video ${tag}`);
		await dialog.getByRole("button", { name: "Add an account" }).click();
		const target = dialog.getByTestId("target-panel");
		await target.getByLabel("Account", { exact: true }).selectOption({
			index: 1,
		});
		await target.getByLabel("Status at the destination").selectOption("public");
		const at = new Date(Date.now() + 30_000);
		at.setSeconds(0, 0);
		at.setMinutes(at.getMinutes() + 1);
		await target.getByLabel("Date and time").fill(localField(at));
		await dialog.getByRole("button", { name: "Save", exact: true }).click();
		await expect(target.getByTestId("target-state")).toHaveText("Draft");
		expect(existsSync(join(folder, slug))).toBe(false);

		await target.getByTestId("target-schedule").click();
		await expect(target.getByTestId("target-state")).toHaveText(
			"Waiting for approval",
		);
		await page.keyboard.press("Escape");

		const pubs = (await api(
			"GET",
			`/publications?project_id=${projectId}`,
		)) as { publications: Publication[] };
		const pub = pubs.publications[0];
		expect(pub?.post).toBe(postId);
		expect(pub?.targets[0]?.network).toBe("blog_files");
		const id = pub?.id ?? "";

		// 4. A person approves it: what is said is exactly what is sent, and nothing is written before.
		await page.goto("/approvals");
		const item = page
			.locator("[data-approval-id]")
			.filter({ hasText: `How I cut a video ${tag}` });
		await expect(item.getByTestId("approval-summary")).toContainText(
			"Blog: files",
		);
		await expect(item.getByTestId("approval-summary")).toContainText(
			`${slug}/index.md`,
		);
		await item.getByRole("button", { name: "Approve" }).click();
		await expect(item).toHaveCount(0);
		expect(existsSync(join(folder, slug))).toBe(false);

		// 5. At its time the scheduler writes the post, once.
		await expect
			.poll(
				async () =>
					((await api("GET", `/publications/${id}`)) as Publication).targets[0]
						?.state,
				{ timeout: 150_000 },
			)
			.toBe("published");
		const written = await readFile(join(folder, slug, "index.md"), "utf8");
		expect(written).toContain(`title: "How I cut a video ${tag}"`);
		expect(written).toContain("draft: false");
		expect(written).toContain("A *short* post.");
		// The payload is the author's own text in the file, as written: no surface interprets it.
		expect(written).toContain(PAYLOAD);

		await page.goto("/publish");
		await expect(page.getByTestId("target-chip")).toContainText("Published", {
			timeout: 30_000,
		});
	});

	test("the destination form and the post editor are in Portuguese too", async ({
		page,
	}) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await signInAsPerson(page);

		await page.goto("/connections");
		const blog = page.getByTestId("blog-section");
		await expect(
			blog.getByRole("heading", { name: "Destinos do blog" }),
		).toBeVisible();
		await blog
			.getByLabel("Destino", { exact: true })
			.selectOption("blog_webhook");
		await expect(blog.getByTestId("blog-about")).toContainText(
			"Endereços desta máquina ou de uma rede privada são recusados",
		);
		await blog
			.getByLabel("Destino", { exact: true })
			.selectOption("blog_webhook_local");
		await expect(blog.getByLabel("Segredo de assinatura")).toBeVisible();
		await expect(blog.getByTestId("blog-about")).toContainText(
			"só para um serviço que você mesmo roda",
		);
	});

	test("a webhook to a private address is refused unless the destination is the local kind", async ({
		page,
	}) => {
		await signInAsPerson(page);
		await page.goto("/connections");
		const blog = page.getByTestId("blog-section");
		await blog
			.getByLabel("Destination", { exact: true })
			.selectOption("blog_webhook");
		await blog.getByLabel("Name").fill("Hook");
		await blog
			.getByLabel("URL that receives the post")
			.fill("https://10.0.0.5/hook");
		await blog.getByLabel("Signing secret").fill("a-long-signing-secret-1234");
		await blog.getByRole("button", { name: "Add destination" }).click();
		await expect(page.getByRole("alert").first()).toContainText(
			"blog_webhook_local",
		);
	});
});
