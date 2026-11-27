import type { Page } from "@playwright/test";
import { expect, test } from "./helpers";

// Epic 101: desktop in-app updater control on the Login screen and the dashboard
// sidebar footer, hidden in plain web browsers and backed by window.vukemDesktop.
async function fakeUpdaterShell(
	page: Page,
	initial: {
		current_version: string;
		available_version?: string;
		auto_update: boolean;
		status: string;
	},
	nextOnCheck?: {
		available_version?: string;
		status: string;
	},
): Promise<void> {
	await page.addInitScript(
		({ init, onCheck }) => {
			const w = window as unknown as Record<string, unknown>;
			let state = { ...init };
			const calls: string[] = [];
			w.__updaterCalls = calls;
			w.vukemDesktop = {
				getUpdateState: () => Promise.resolve({ ...state }),
				checkForUpdates: () => {
					calls.push("check");
					if (onCheck) {
						state = { ...state, ...onCheck };
					}
					return Promise.resolve({ ...state });
				},
				installUpdate: () => {
					calls.push("install");
					state = { ...state, status: "installing" };
					return Promise.resolve({ ...state });
				},
				setAutoUpdate: (enabled: boolean) => {
					calls.push(`auto:${String(enabled)}`);
					state = { ...state, auto_update: enabled };
					return Promise.resolve({ ...state });
				},
			};
		},
		{ init: initial, onCheck: nextOnCheck },
	);
}

test.describe("desktop updater control", () => {
	test("is completely hidden in a plain web browser", async ({ page }) => {
		await page.goto("/");
		await expect(page.getByTestId("desktop-update-sidebar")).toHaveCount(0);
		await expect(page.getByTestId("desktop-update-action")).toHaveCount(0);
	});

	test("renders in the sidebar footer inside Stuarty Desktop, checks updates and toggles auto-update", async ({
		page,
	}) => {
		await fakeUpdaterShell(
			page,
			{
				current_version: "0.61.0",
				auto_update: false,
				status: "idle",
			},
			{
				available_version: "0.62.0",
				status: "available",
			},
		);
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto("/");

		const section = page.getByTestId("desktop-update-sidebar");
		await expect(section).toBeVisible();

		const toggle = section.getByTestId("desktop-update-auto");
		await expect(toggle).not.toBeChecked();
		await toggle.click();
		await expect(toggle).toBeChecked();

		const action = section.getByTestId("desktop-update-action");
		await action.click();
		await expect(action).toContainText("Update to v0.62.0");
		await expect(section.getByTestId("desktop-update-status")).toContainText(
			"v0.62.0",
		);

		await action.click();
		await expect(section.getByTestId("desktop-update-status")).toContainText(
			"Installing update",
		);

		const calls = await page.evaluate(
			() => (window as unknown as { __updaterCalls: string[] }).__updaterCalls,
		);
		expect(calls).toEqual(["auto:true", "check", "install"]);
	});

	test("shows offline state cleanly when disconnected", async ({ page }) => {
		await fakeUpdaterShell(
			page,
			{
				current_version: "0.61.0",
				auto_update: false,
				status: "idle",
			},
			{ status: "offline" },
		);
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.goto("/");

		const section = page.getByTestId("desktop-update-sidebar");
		await section.getByTestId("desktop-update-action").click();
		await expect(section.getByTestId("desktop-update-status")).toContainText(
			"Offline",
		);
	});
});
