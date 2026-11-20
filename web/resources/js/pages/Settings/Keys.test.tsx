import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { ApiError } from "../../editor/api/errors";
import type { ApiKey } from "../../editor/api/types";
import SettingsKeys, { KeyRow, reportOrLogin } from "./Keys";

vi.mock("@inertiajs/react", () => ({ Head: () => null }));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.keys.created": "Created {date}",
	"ui.keys.revoked": "Revoked",
	"ui.keys.revoke": "Revoke",
};

const key = (extra: Partial<ApiKey> = {}): ApiKey => ({
	key_id: "key_1",
	name: PAYLOAD,
	scopes: ["read"],
	project_id: null,
	created_at: "2026-11-15T12:00:00Z",
	last_used_at: null,
	expires_at: null,
	revoked_at: null,
	...extra,
});

function markup(apiKey: ApiKey): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<ul>
				<KeyRow apiKey={apiKey} onRevoke={() => undefined} />
			</ul>
		</I18nProvider>,
	);
}

// SDD-007 section 8.5, threat T-22: the name of a key is written by whoever
// created it and reaches this page as text, never as markup. The page cannot be
// opened in the local-mode Playwright run (it answers 404 there).
describe("keys page renders the key name as text", () => {
	test.each([
		["active", key()],
		["revoked", key({ revoked_at: "2026-11-15T13:00:00Z" })],
	])("%s key", (_label, apiKey) => {
		const html = markup(apiKey);
		expect(html).toContain("&lt;img src=x onerror=window.__pwned=1&gt;");
		expect(html).not.toContain("<img");
	});
});

// SDD-020 section 47: the key of the read API of the blog is made on this page, from a privileged session, and the
// scope is offered with the five others.
describe("the scopes a new key may hold", () => {
	test("the form offers posts:read", () => {
		const html = renderToString(
			<I18nProvider messages={messages} locale="en" locales={["en"]}>
				<SettingsKeys />
			</I18nProvider>,
		);
		for (const scope of ["read", "edit", "media", "render", "admin"]) {
			expect(html).toContain(`>${scope}</label>`);
		}
		expect(html).toContain(">posts:read</label>");
	});
});

describe("a failed call of the keys page", () => {
	test("E_UNAUTHORIZED goes to the login, back to this page", () => {
		const show = vi.fn();
		const goTo = vi.fn();
		const error = new ApiError(401, { code: "E_UNAUTHORIZED", message: "no" });
		reportOrLogin(error, show, goTo);
		expect(goTo).toHaveBeenCalledWith("/login?next=%2Fsettings%2Fkeys");
		expect(show).not.toHaveBeenCalled();
	});

	test("any other failure is shown on the page", () => {
		const show = vi.fn();
		const goTo = vi.fn();
		reportOrLogin(
			new ApiError(403, { code: "E_FORBIDDEN", message: "no" }),
			show,
			goTo,
		);
		expect(goTo).not.toHaveBeenCalled();
		expect(show).toHaveBeenCalledTimes(1);
	});
});
