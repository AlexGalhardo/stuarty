import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import GoogleCallback, { GOOGLE_NEXT_KEY } from "./GoogleCallback";
import source from "./GoogleCallback.tsx?raw";

vi.mock("@inertiajs/react", () => ({
	Head: () => null,
	Link: ({ children }: { children: unknown }) => children,
}));

const messages = {
	"ui.google.callback_title": "Signing in with Google",
	"ui.google.callback_working": "Finishing the sign-in",
};

describe("the Google redirect page", () => {
	test("draws a working state and nothing from the address", () => {
		const html = renderToString(
			<I18nProvider messages={messages} locale="en" locales={["en"]}>
				<GoogleCallback />
			</I18nProvider>,
		);
		expect(html).toContain("Finishing the sign-in");
		expect(html).toContain('data-google-phase="working"');
	});

	test("hands the state to the server, keeps no credential and goes only to a path of this site", () => {
		expect(source).toContain("googleComplete");
		expect(source).toContain("safeNext(");
		expect(source).not.toMatch(/localStorage|document\.cookie/);
		// Only the place the person was going is kept, in the tab, never a token or a code.
		expect(GOOGLE_NEXT_KEY).toBe("stuarty.google.next");
		expect(source).not.toMatch(/setItem\([^)]*(code|state|token)/i);
	});
});
