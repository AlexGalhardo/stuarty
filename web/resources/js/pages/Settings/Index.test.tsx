import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import SettingsIndex from "./Index";

const page = vi.hoisted(() => ({
	account: { state: "unlocked", username: "ana" },
}));

vi.mock("@inertiajs/react", () => ({
	Head: () => null,
	Link: ({ children }: { children: unknown }) => children,
	router: { reload: () => undefined },
	usePage: () => ({ props: { account: page.account } }),
}));

const messages = {
	"ui.settings.title": "Settings",
	"ui.settings.intro": "Your account.",
	"ui.settings.sections": "Sections of the settings",
	"ui.settings.preferences": "Preferences",
	"ui.settings.about": "About",
	"ui.settings.language": "Language",
	"ui.settings.theme": "Theme",
	"ui.settings.loading": "Loading…",
	"ui.brand.title": "Brand kit",
	"ui.brand.intro": "Colours and fonts.",
	"ui.brand.open": "Open the brand kit",
	"ui.account.settings.title": "Account",
	"ui.settings.transcription.title": "Transcription",
};

function markup(): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<SettingsIndex />
		</I18nProvider>,
	);
}

describe("the settings page", () => {
	test("lists its sections and links each one to a section that is on the page", () => {
		const html = markup();
		const nav = html.match(/<nav[^>]*>.*?<\/nav>/s)?.[0] ?? "";
		expect(nav).toContain('aria-label="Sections of the settings"');
		const targets = [...nav.matchAll(/href="#([^"]+)"/g)].map((m) => m[1]);
		expect(targets).toEqual([
			"settings-preferences",
			"settings-brand",
			"settings-account",
			"settings-transcription",
			"settings-about",
		]);
		for (const id of targets) expect(html).toContain(`id="${id}`);
	});

	test("wraps the list of sections instead of hiding some of them in a sideways scroll", () => {
		const nav = markup().match(/<nav[^>]*>/)?.[0] ?? "";
		expect(nav).toContain("flex-wrap");
		expect(nav).not.toContain("overflow-x");
	});

	test("has one h1", () => {
		expect(markup().match(/<h1/g)).toHaveLength(1);
	});

	test("leaves the account out of the list while it is not unlocked", () => {
		page.account = { state: "locked", username: "ana" };
		const html = markup();
		page.account = { state: "unlocked", username: "ana" };
		expect(html).not.toContain('href="#settings-account"');
	});
});
