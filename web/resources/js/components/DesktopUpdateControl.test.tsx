import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import type { DesktopBridge, DesktopUpdateState } from "@/lib/desktopBridge";
import { I18nProvider } from "@/lib/i18n";
import { DesktopUpdateControl } from "./DesktopUpdateControl";

const messages: Record<string, string> = {
	"ui.desktop_update.title": "Desktop updates",
	"ui.desktop_update.check": "Update to latest",
	"ui.desktop_update.install": "Update to v{version}",
	"ui.desktop_update.auto_label": "Auto-update when online",
	"ui.desktop_update.status.available":
		"Version v{version} is ready to install.",
	"ui.desktop_update.status.up_to_date":
		"You are on the latest stable version.",
	"ui.desktop_update.status.checking": "Checking for updates...",
	"ui.desktop_update.status.downloading": "Downloading update ({progress}%)...",
	"ui.desktop_update.status.installing": "Installing update...",
	"ui.desktop_update.status.offline":
		"Offline. Connect to the internet to check for updates.",
	"ui.desktop_update.status.unconfigured":
		"Automatic updates are not enabled in this build.",
	"ui.desktop_update.status.error": "Could not check for updates.",
};

const fakeBridge: DesktopBridge = {
	getUpdateState: async () => ({}),
	checkForUpdates: async () => ({}),
	installUpdate: async () => ({}),
	setAutoUpdate: async () => ({}),
};

function draw(
	variant: "card" | "sidebar",
	bridge: DesktopBridge | null,
	initialState?: DesktopUpdateState,
	collapsed = false,
): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<TooltipProvider>
				<DesktopUpdateControl
					variant={variant}
					collapsed={collapsed}
					bridgeOverride={bridge}
					initialState={initialState}
				/>
			</TooltipProvider>
		</I18nProvider>,
	);
}

describe("DesktopUpdateControl", () => {
	test("renders nothing in a plain web browser without the desktop bridge", () => {
		expect(draw("card", null)).toBe("");
		expect(draw("sidebar", null)).toBe("");
	});

	test("renders update button and auto-update toggle on Login card and sidebar footer", () => {
		const html = draw("card", fakeBridge, {
			current_version: "0.61.0",
			auto_update: true,
			status: "idle",
		});
		expect(html).toContain('data-testid="desktop-update-card"');
		expect(html).toContain("Update to latest");
		expect(html).toContain("Auto-update when online");
		expect(html).toContain('data-testid="desktop-update-auto"');
	});

	test("shows versioned install button and status when an update is available", () => {
		const html = draw("sidebar", fakeBridge, {
			current_version: "0.61.0",
			available_version: "0.62.0",
			auto_update: false,
			status: "available",
		});
		expect(html).toContain('data-testid="desktop-update-sidebar"');
		expect(html).toContain("Update to v0.62.0");
		expect(html).toContain("Version v0.62.0 is ready to install.");
	});

	test("shows offline message cleanly and collapses to an icon button in collapsed sidebar", () => {
		const offline = draw("card", fakeBridge, {
			current_version: "0.61.0",
			auto_update: false,
			status: "offline",
		});
		expect(offline).toContain("Offline. Connect to the internet");

		const rail = draw(
			"sidebar",
			fakeBridge,
			{
				current_version: "0.61.0",
				auto_update: false,
				status: "idle",
			},
			true,
		);
		expect(rail).toContain('data-testid="desktop-update-action"');
		expect(rail).not.toContain('data-testid="desktop-update-auto"');
	});
});
