import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import BlogSection, {
	BLOG_KINDS,
	type BlogForm,
	blogRequest,
	EMPTY_FORM,
} from "./BlogSection";

vi.mock("@inertiajs/react", () => ({ Head: () => null, Link: () => null }));

const form = (patch: Partial<BlogForm>): BlogForm => ({
	...EMPTY_FORM,
	label: " My blog ",
	...patch,
});

describe("the request of a blog destination", () => {
	test("files: the placeholder endpoint and the folder and template as the secret", () => {
		const request = blogRequest(
			form({ kind: "blog_files", folder: " /srv/site/content ", zip: true }),
		);
		expect(request.kind).toBe("blog_files");
		expect(request.label).toBe("My blog");
		expect(request.endpoint).toBe("http://localhost");
		expect(JSON.parse(request.secret)).toEqual({
			folder: "/srv/site/content",
			template: "astro",
			zip: true,
		});
	});

	test("a custom template carries its text, a preset does not", () => {
		const custom = blogRequest(
			form({
				kind: "blog_files",
				folder: "/x",
				template: "custom",
				frontMatter: "---\ntitle: {{title}}\n---",
			}),
		);
		expect(JSON.parse(custom.secret)).toMatchObject({
			template: "custom",
			front_matter: "---\ntitle: {{title}}\n---",
		});
		const preset = blogRequest(
			form({
				kind: "blog_files",
				folder: "/x",
				template: "hugo",
				frontMatter: "left over",
			}),
		);
		expect(JSON.parse(preset.secret)).toEqual({
			folder: "/x",
			template: "hugo",
		});
	});

	test("WordPress: the address of the site and the application password", () => {
		const request = blogRequest(
			form({
				kind: "blog_wordpress",
				site: " https://blog.example.com ",
				user: " editor ",
				applicationPassword: "abcd efgh ijkl",
			}),
		);
		expect(request.endpoint).toBe("https://blog.example.com");
		expect(JSON.parse(request.secret)).toEqual({
			user: "editor",
			application_password: "abcd efgh ijkl",
		});
	});

	test("the two webhooks differ in their kind only", () => {
		const base = {
			hook: "https://hooks.example.com/stuarty",
			signingSecret: "a-long-signing-secret",
			mediaBase: "https://stuarty.example.com",
		};
		const publicHook = blogRequest(form({ kind: "blog_webhook", ...base }));
		const localHook = blogRequest(
			form({ kind: "blog_webhook_local", ...base }),
		);
		expect(publicHook.kind).toBe("blog_webhook");
		expect(localHook.kind).toBe("blog_webhook_local");
		expect(localHook.endpoint).toBe(publicHook.endpoint);
		expect(JSON.parse(localHook.secret)).toEqual({
			signing_secret: "a-long-signing-secret",
			media_base_url: "https://stuarty.example.com",
		});
		const bare = blogRequest(
			form({ kind: "blog_webhook", ...base, mediaBase: "" }),
		);
		expect(JSON.parse(bare.secret)).toEqual({
			signing_secret: "a-long-signing-secret",
		});
	});

	test("the read API has no settings", () => {
		const request = blogRequest(form({ kind: "blog_api" }));
		expect(request.endpoint).toBe("http://localhost");
		expect(request.secret).toBe("{}");
	});
});

const messages: Record<string, string> = {
	"ui.connections.blog.title": "Blog destinations",
	"ui.connections.blog.intro": "Where a blog post goes.",
	"ui.connections.blog.form": "New blog destination",
	"ui.connections.blog.kind": "Destination",
	"ui.connections.label": "Name",
	"ui.connections.blog.add": "Add destination",
	"ui.connections.blog.folder": "Folder",
	"ui.connections.blog.folder_hint": "An absolute path.",
	"ui.connections.blog.template": "Front matter of the files",
	"ui.connections.blog.zip": "Write a zip",
	"ui.connections.blog.about.blog_files": "Nothing leaves this machine.",
	...Object.fromEntries(
		BLOG_KINDS.map((k) => [`ui.connections.blog.kinds.${k}`, k]),
	),
	...Object.fromEntries(
		["astro", "hugo", "jekyll", "custom"].map((n) => [
			`ui.connections.blog.templates.${n}`,
			n,
		]),
	),
};

describe("the section of the blog destinations", () => {
	test("offers the five kinds and starts on the files form", () => {
		const html = renderToString(
			<I18nProvider locale="en" messages={messages}>
				<BlogSection onChanged={() => undefined} />
			</I18nProvider>,
		);
		for (const kind of BLOG_KINDS) expect(html).toContain(`value="${kind}"`);
		expect(html).toContain("Folder");
		expect(html).toContain("Nothing leaves this machine.");
		expect(html).not.toContain("Application password");
		// The button waits until the form says what it needs.
		expect(html).toMatch(/<button[^>]*disabled[^>]*>Add destination/);
	});
});
