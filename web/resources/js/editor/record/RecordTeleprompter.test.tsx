import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import type { DesktopBridge } from "@/lib/desktopBridge";
import { I18nProvider } from "@/lib/i18n";
import en from "../../../lang/en.json";
import { TeleprompterControls } from "./RecordTeleprompter";

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

function draw(bridge: DesktopBridge | null): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<TeleprompterControls
				bridge={bridge}
				projectId="prj_7k2m9x4q1b"
				label="Teleprompter"
				panel={(show) => (show ? <p>panel</p> : null)}
			/>
		</I18nProvider>,
	);
}

describe("the teleprompter of the record dialog", () => {
	test("on the web is a panel of the page, and says a capture of the tab records it", () => {
		const html = draw(null);
		expect(html).toContain("Show the teleprompter here");
		expect(html).toContain("A capture of this tab or screen records it.");
		expect(html).not.toContain("Open the teleprompter window");
	});

	test("on the desktop is a window of its own, and says where the system keeps it out of a capture", () => {
		const html = draw({
			openTeleprompter: async () => undefined,
			closeTeleprompter: async () => undefined,
		});
		expect(html).toContain("Open the teleprompter window");
		expect(html).toContain("Windows 10 (version 2004) or later and on macOS");
		expect(html).not.toContain("Show the teleprompter here");
	});

	test("a desktop with no teleprompter member is the web form", () => {
		expect(draw({ pickFiles: async () => [] })).toContain(
			"Show the teleprompter here",
		);
	});
});
