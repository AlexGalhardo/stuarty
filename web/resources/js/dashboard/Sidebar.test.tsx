import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import en from "../../lang/en.json";
import Sidebar from "./Sidebar";

// The catalogue the server sends is the flattened "ui." tree; the tests only
// need the keys the sidebar reads.
function flat(
	tree: Record<string, unknown>,
	prefix: string,
	out: Record<string, string> = {},
): Record<string, string> {
	for (const [key, value] of Object.entries(tree)) {
		if (typeof value === "string") out[`${prefix}${key}`] = value;
		else flat(value as Record<string, unknown>, `${prefix}${key}.`, out);
	}
	return out;
}
const messages = flat(en.ui, "ui.");

function draw(
	url: string,
	user: { id: string; email: string; role: string } | null,
): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
			<Sidebar url={url} appName="Stuarty" user={user} />
		</I18nProvider>,
	);
}

describe("Sidebar", () => {
	test("is a navigation landmark with the eleven sections in order", () => {
		const html = draw("/", null);
		expect(html).toContain('<nav aria-label="Main navigation"');
		const labels = [
			...html.matchAll(
				/data-testid="nav-[a-z-]+"[^>]*>.*?<span[^>]*>([^<]+)<\/span>/g,
			),
		].map((m) => m[1]);
		expect(labels).toEqual([
			"Projects",
			"Edit Video",
			"Edit Image",
			"Write",
			"Visual Assets",
			"Video Assets",
			"Publish",
			"Skills",
			"Prompts",
			"Memory",
			"Connections",
		]);
	});

	test("announces only the current section", () => {
		const html = draw("/publish", null);
		expect(html.match(/aria-current="page"/g)).toHaveLength(1);
		expect(html).toMatch(/aria-current="page"[^>]*data-testid="nav-publish"/);
	});

	test("the home is current on the root and no section is", () => {
		const html = draw("/", null);
		expect(html.match(/aria-current="page"/g)).toHaveLength(1);
		expect(html).toMatch(/aria-current="page"[^>]*>.*Stuarty/s);
		expect(html).not.toMatch(/aria-current="page"[^>]*data-testid="nav-/);
	});

	test("local mode shows Local and no sign out", () => {
		const html = draw("/", null);
		expect(html).toContain("Local");
		expect(html).not.toContain("Log out");
		expect(html).toContain('href="/settings"');
	});

	test("network mode shows the email, the keys and the sign out", () => {
		const html = draw("/settings/keys", {
			id: "usr_1",
			email: "ana@example.com",
			role: "member",
		});
		expect(html).toContain("ana@example.com");
		expect(html).toContain("Log out");
		expect(html).toContain('href="/settings/keys"');
	});

	test("starts expanded and offers the collapse button", () => {
		const html = draw("/", null);
		expect(html).toContain('data-state="expanded"');
		expect(html).toContain('aria-label="Collapse sidebar"');
		expect(html).toContain('aria-expanded="true"');
	});

	test("carries the theme toggle", () => {
		expect(draw("/", null)).toContain("Switch to dark theme");
	});
});
