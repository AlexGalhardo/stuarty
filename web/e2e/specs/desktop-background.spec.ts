import type { Page } from "@playwright/test";
import { expect, test } from "./helpers";

// Epic 21 (SDD-012 section 21.3 item 7): the desktop shell keeps a post scheduled for later going out with the
// window closed. The settings page draws the Background section only where the shell defines
// window.vukemDesktop.getBackground and setBackground; the shell keeps the choice, the page only shows and sends
// it. The shell is faked here by a bridge that remembers what it was sent.

interface Sent {
	keep_running: boolean;
	start_at_sign_in: boolean;
	asked: boolean;
}

async function fakeShell(
	page: Page,
	options: { refuseStart?: boolean } = {},
): Promise<void> {
	await page.addInitScript((refuse) => {
		const w = window as unknown as Record<string, unknown>;
		let held = { keep_running: false, start_at_sign_in: false, asked: false };
		const sent: unknown[] = [];
		w.__background = sent;
		w.vukemDesktop = {
			getBackground: () => Promise.resolve({ ...held }),
			setBackground: (s: typeof held) => {
				sent.push(s);
				if (refuse && s.start_at_sign_in) {
					held = { ...s, start_at_sign_in: false };
					return Promise.resolve({ ...held, error: "access is denied" });
				}
				held = { ...s };
				return Promise.resolve({ ...held });
			},
		};
	}, options.refuseStart === true);
}

function sentOf(page: Page): Promise<Sent[]> {
	return page.evaluate(
		() => (window as unknown as { __background: Sent[] }).__background,
	);
}

test.describe("desktop background mode", () => {
	test("a plain browser has no Background section", async ({ page }) => {
		await page.goto("/settings");
		await expect(
			page.getByRole("heading", { name: "Settings", level: 1 }),
		).toBeVisible();
		await expect(page.getByTestId("settings-background")).toHaveCount(0);
	});

	test("the shell shows the section, keeps nothing on by itself and says what it cannot do", async ({
		page,
	}) => {
		await fakeShell(page);
		await page.goto("/settings");
		const section = page.getByTestId("settings-background");
		await expect(section).toBeVisible();
		await expect(
			section.getByRole("switch", { name: "Keep running in the background" }),
		).not.toBeChecked();
		await expect(
			section.getByRole("switch", { name: "Start when I sign in" }),
		).toBeDisabled();
		await expect(section).toContainText(
			"only if the computer is on and you are signed in",
		);
		await expect(section).toContainText("waits for your decision");
		await expect(section).toContainText(
			"Quit in the tray icon stops publishing",
		);
		expect(await sentOf(page)).toEqual([]);
	});

	test("turning the options on and off sends the choice to the shell", async ({
		page,
	}) => {
		await fakeShell(page);
		await page.goto("/settings");
		const keep = page.getByRole("switch", {
			name: "Keep running in the background",
		});
		const start = page.getByRole("switch", { name: "Start when I sign in" });
		await keep.click();
		await expect(keep).toBeChecked();
		await expect(start).toBeEnabled();
		await start.click();
		await expect(start).toBeChecked();
		await keep.click();
		await expect(keep).not.toBeChecked();
		await expect(start).not.toBeChecked();
		expect(await sentOf(page)).toEqual([
			{ keep_running: true, start_at_sign_in: false, asked: true },
			{ keep_running: true, start_at_sign_in: true, asked: true },
			{ keep_running: false, start_at_sign_in: false, asked: true },
		]);
	});

	test("a refusal of the system leaves start at sign-in off and says why", async ({
		page,
	}) => {
		await fakeShell(page, { refuseStart: true });
		await page.goto("/settings");
		await page
			.getByRole("switch", { name: "Keep running in the background" })
			.click();
		const start = page.getByRole("switch", { name: "Start when I sign in" });
		await start.click();
		await expect(
			page.getByRole("alert").filter({ hasText: "access is denied" }),
		).toBeVisible();
		await expect(start).not.toBeChecked();
	});

	test("the section follows the language of the page", async ({ page }) => {
		await fakeShell(page);
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await page.goto("/settings");
		const section = page.getByTestId("settings-background");
		await expect(
			section.getByRole("heading", { name: "Segundo plano" }),
		).toBeVisible();
		await expect(
			section.getByRole("switch", {
				name: "Continuar rodando em segundo plano",
			}),
		).toBeVisible();
	});
});
