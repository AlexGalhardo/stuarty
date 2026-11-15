import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import Login from "./Login";
import source from "./Login.tsx?raw";

// Head needs the Inertia app; the page content does not.
vi.mock("@inertiajs/react", () => ({ Head: () => null }));

const messages = {
	"ui.login.title": "Log in",
	"ui.login.email": "Email",
	"ui.login.password": "Password",
	"ui.login.submit": "Log in",
};

function markup(): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<Login />
		</I18nProvider>,
	);
}

// WCAG 2.2 criterion 3.3.8 Accessible Authentication (Minimum), SDD-012 section
// 18.6: the login asks for no cognitive function test, and the browser and a
// password manager can fill and paste both fields.
describe("login accessible authentication", () => {
	const html = markup();

	test("both fields name their purpose for the browser", () => {
		expect(html).toMatch(/<input[^>]*id="login-email"[^>]*>/);
		expect(html.match(/<input[^>]*id="login-email"[^>]*>/)?.[0]).toContain(
			`autoComplete="username"`,
		);
		expect(html).toContain(`autoComplete="current-password"`);
	});

	test("both fields have a visible label", () => {
		expect(html).toContain('for="login-email"');
		expect(html).toContain('for="login-password"');
	});

	test("nothing blocks paste or turns autofill off, and there is no puzzle", () => {
		expect(source).not.toMatch(/onPaste|onCopy|onCut|onDrop/);
		expect(source).not.toMatch(/autoComplete=["']off["']/);
		expect(source).not.toMatch(/captcha|recaptcha|hcaptcha|turnstile/i);
	});
});
