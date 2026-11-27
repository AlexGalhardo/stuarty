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
	test("offers Logout to a person only: a launch link cannot lock the studio", () => {
		const account = { state: "unlocked", username: "stuart" } as const;
		const of = (person: boolean) =>
			renderToString(
				<I18nProvider messages={messages} locale="en" locales={["en"]}>
					<Sidebar
						url="/"
						appName="Stuarty"
						user={null}
						account={account}
						person={person}
					/>
				</I18nProvider>,
			);
		expect(of(true)).toContain('data-testid="account-logout"');
		expect(of(false)).not.toContain('data-testid="account-logout"');
	});

	test("is a navigation landmark: Home, then the five groups with their items in order", () => {
		const html = draw("/", null);
		expect(html).toContain('<nav aria-label="Main navigation"');
		const labels = [
			...html.matchAll(
				/data-testid="nav-[a-z-]+"[^>]*>.*?<span[^>]*>([^<]+)<\/span>/g,
			),
		].map((m) => m[1]);
		expect(labels).toEqual([
			"Home",
			"Projects",
			"Writing",
			"Video editor",
			"Image editor",
			"History",
			"Videos",
			"Images",
			"Sounds",
			"Brand kit",
			"Skills",
			"Prompts",
			"Memory",
			"Approvals",
			"Publishing",
			"Connections",
			"Webhooks",
		]);
		const groups = [
			...html.matchAll(/id="nav-group-[a-z]+"[^>]*>([^<]+)<\/p>/g),
		].map((m) => m[1]);
		expect(groups).toEqual(["Create", "Assets", "Agent", "Publish", "Connect"]);
	});

	test("the keys link is a link of the Connect group in network mode only", () => {
		const user = { id: "usr_1", email: "ana@example.com", role: "member" };
		expect(draw("/", user)).toContain('data-testid="nav-keys"');
		expect(draw("/", null)).not.toContain('data-testid="nav-keys"');
	});

	test("announces only the current section", () => {
		const html = draw("/publish", null);
		expect(html.match(/aria-current="page"/g)).toHaveLength(1);
		expect(html).toMatch(/aria-current="page"[^>]*data-testid="nav-publish"/);
	});

	test("Approvals and the brand kit are current on their own pages", () => {
		expect(draw("/approvals", null)).toMatch(
			/aria-current="page"[^>]*data-testid="nav-approvals"/,
		);
		expect(draw("/settings/brand-kit", null)).toMatch(
			/aria-current="page"[^>]*data-testid="nav-brand-kit"/,
		);
	});

	test("the home is current on the root and no section is", () => {
		const html = draw("/", null);
		expect(html.match(/aria-current="page"/g)).toHaveLength(2);
		expect(html).toMatch(/aria-current="page"[^>]*data-testid="nav-home"/);
		const current = [
			...html.matchAll(/aria-current="page"[^>]*data-testid="nav-([a-z-]+)"/g),
		].map((m) => m[1]);
		expect(current).toEqual(["home"]);
	});

	test("the Approvals item carries the count as a word for a reader and a number for the eye", () => {
		const html = renderToString(
			<I18nProvider messages={messages} locale="en" locales={["en"]}>
				<Sidebar url="/" appName="Stuarty" user={null} pendingApprovals={3} />
			</I18nProvider>,
		);
		expect(html).toContain("3 waiting for you");
		expect(html).toMatch(
			/aria-hidden="true"[^>]*data-testid="nav-count"[^>]*>3</,
		);
	});

	test("shows no count when nothing waits", () => {
		expect(draw("/", null)).not.toContain("nav-count");
	});

	test("the footer holds the settings link, the version slot and the mode", () => {
		const html = draw("/", null);
		expect(html).toContain('data-testid="sidebar-mode"');
		expect(html).toContain("Local mode");
	});

	test("the rail keeps every item's name and hides its group headings and version", () => {
		const html = renderToString(
			<I18nProvider messages={messages} locale="en" locales={["en"]}>
				<Sidebar url="/" appName="Stuarty" user={null} form="rail" />
			</I18nProvider>,
		);
		expect(html).toContain('data-form="rail"');
		expect(html).toContain('data-state="collapsed"');
		expect(html).toMatch(/<span class="sr-only">Projects<\/span>/);
		expect(html).toMatch(/id="nav-group-create"[^>]*class="[^"]*sr-only/);
		expect(html).not.toContain('data-testid="sidebar-version"');
		expect(html).not.toContain('data-testid="sidebar-mode"');
	});

	test("the drawer draws no column: the top bar opens it", () => {
		const html = renderToString(
			<I18nProvider messages={messages} locale="en" locales={["en"]}>
				<Sidebar url="/" appName="Stuarty" user={null} form="drawer" />
			</I18nProvider>,
		);
		expect(html).not.toContain('data-testid="sidebar"');
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
		// Collapsed, an item shows only its icon: the keys link needs one.
		expect(html).toMatch(/href="\/settings\/keys"[^>]*><svg/);
	});

	test("the mark, the navigation, the profile and the controls are one block with no banner of its own", () => {
		const html = draw("/", null);
		expect(html).toMatch(/^<div [^>]*data-testid="sidebar"/);
		expect(html).not.toContain("<header");
	});

	test("starts expanded and offers the collapse button", () => {
		const html = renderToString(
			<I18nProvider messages={messages} locale="en" locales={["en"]}>
				<Sidebar
					url="/"
					appName="Stuarty"
					user={null}
					onToggle={() => undefined}
				/>
			</I18nProvider>,
		);
		expect(html).toContain('data-state="expanded"');
		expect(html).toContain('aria-label="Collapse sidebar"');
		expect(html).toContain('aria-expanded="true"');
	});

	test("carries the theme toggle", () => {
		expect(draw("/", null)).toContain("Switch to dark theme");
	});
});
