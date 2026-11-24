import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { expect, type Page, test } from "@playwright/test";
import { home } from "../support/accountTargets";

// The first run, the recovery key, the sign-in and Logout of the local account (ADR-0052, SDD-012 section 2.1.1),
// on a server whose workspace has no account (playwright.config.ts, the project "account"). The specs run in
// order: each one starts from what the one before left.
test.describe.configure({ mode: "serial" });

const accountHome = `${home}-account`;
const PASSWORD = "Correct-Horse-9";
const NEW_PASSWORD = "Another-Pass-7!";

let recoveryKey = "";
let replacedRecoveryKey = "";

// `stuarty open`: the local token asks for a launch code and the browser redeems it. It is an editor session and
// not a person; the account pages need nothing more.
async function open(page: Page): Promise<void> {
	const token = (await readFile(join(accountHome, "token"), "utf8")).trim();
	const launch = await page.request.post("/api/v1/system/launch", {
		headers: { Authorization: `Bearer ${token}` },
	});
	expect(launch.ok()).toBe(true);
	const { url } = (await launch.json()) as { url: string };
	await page.goto(url);
}

async function signIn(page: Page, password: string): Promise<void> {
	await page.getByLabel("Username or e-mail").fill("stuart");
	await page.getByLabel("Password", { exact: true }).fill(password);
	await page.getByRole("button", { name: "Sign in" }).click();
}

test("a new workspace shows the page that creates the account before anything else", async ({
	page,
}) => {
	await open(page);
	await expect(page).toHaveURL(/\/account\/create$/);
	await expect(
		page.getByRole("heading", { name: "Create your account" }),
	).toBeVisible();
	await page.goto("/projects");
	await expect(page).toHaveURL(/\/account\/create$/);
});

test("each refused field is explained under it, and the eye shows the password", async ({
	page,
}) => {
	await open(page);
	await page.getByLabel("Username", { exact: true }).fill("AB");
	await page.getByLabel("E-mail").fill("x");
	await page.getByLabel("Password", { exact: true }).fill("abc");
	await page.getByLabel("Confirm the password").fill("abd");

	const password = page.getByLabel("Password", { exact: true });
	const eye = page.getByTestId("account-password-eye");
	await expect(password).toHaveAttribute("type", "password");
	await eye.click();
	await expect(password).toHaveAttribute("type", "text");
	await expect(eye).toHaveAttribute("aria-pressed", "true");
	await eye.click();
	await expect(password).toHaveAttribute("type", "password");

	const confirmEye = page.getByTestId("account-confirmation-eye");
	await confirmEye.click();
	await expect(page.getByLabel("Confirm the password")).toHaveAttribute(
		"type",
		"text",
	);

	await page.getByRole("button", { name: "Create account" }).click();
	await expect(page.locator("#account-username-errors")).toContainText(
		"too short",
	);
	await expect(page.locator("#account-email-errors")).toContainText(
		"too short",
	);
	await expect(page.locator("#account-password-errors")).toContainText(
		"too short",
	);
	await expect(page.locator("#account-confirmation-errors")).toContainText(
		"not the same",
	);
	await expect(page.getByLabel("Username", { exact: true })).toHaveAttribute(
		"aria-invalid",
		"true",
	);
});

test("the account is created, the recovery key shown once and the sign-in follows", async ({
	page,
}) => {
	await open(page);
	await page.getByLabel("Username", { exact: true }).fill("stuart");
	await page.getByLabel("E-mail").fill("stuart@example.com");
	await page.getByLabel("Password", { exact: true }).fill(PASSWORD);
	await page.getByLabel("Confirm the password").fill(PASSWORD);
	await page.getByRole("button", { name: "Create account" }).click();

	await expect(
		page.getByRole("heading", { name: "Save your recovery key" }),
	).toBeVisible();
	await expect(page.getByText(/cannot be recovered/)).toBeVisible();
	recoveryKey = (await page.getByTestId("recovery-key").textContent()) ?? "";
	expect(recoveryKey).toMatch(/^([0-9A-Z]{4}-){12}[0-9A-Z]{1,4}$/);

	const next = page.getByRole("button", { name: "Continue to sign in" });
	await expect(next).toBeDisabled();
	await page.getByLabel("I saved my recovery key").check();
	await expect(next).toBeEnabled();
	await next.click();
	await expect(page).toHaveURL(/\/account\/signin$/);

	// The key is gone with the page: it is in no storage and the account cannot be created a second time.
	expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(
		recoveryKey,
	);
	expect(
		await page.evaluate(() => JSON.stringify(sessionStorage)),
	).not.toContain(recoveryKey);
});

test("a wrong password signs nobody in and the right one opens the dashboard", async ({
	page,
}) => {
	await page.goto("/account/signin");
	await signIn(page, NEW_PASSWORD);
	await expect(page.getByRole("alert")).toBeVisible();
	await expect(page).toHaveURL(/\/account\/signin$/);

	await signIn(page, PASSWORD);
	await expect(page).toHaveURL(/\/$/);
	await expect(page.getByTestId("sidebar")).toBeVisible();
	// The sidebar has no language dropdown any more, and the footer has Settings and Logout.
	await expect(page.getByTestId("sidebar").getByRole("combobox")).toHaveCount(
		0,
	);
	await expect(page.getByTestId("account-logout")).toBeVisible();
	await expect(page.getByTestId("profile-name")).toContainText("stuart");
});

test("Settings shows who is signed in and changes the password", async ({
	page,
}) => {
	await page.goto("/account/signin");
	await signIn(page, PASSWORD);
	await expect(page).toHaveURL(/\/$/);
	await page.goto("/settings");
	const section = page.getByTestId("settings-account");
	await expect(section.getByTestId("settings-username")).toHaveText("stuart");

	await section.getByLabel("Lock after being idle").selectOption("15");
	await expect(section.getByRole("status")).toContainText("saved");

	await section.getByLabel("Current password").fill(PASSWORD);
	await section.getByLabel("New password").fill("weak");
	await section.getByLabel("Confirm the password").fill("weak");
	await section.getByRole("button", { name: "Change password" }).click();
	await expect(section.locator("#settings-new-errors")).toContainText(
		"too short",
	);

	await section.getByLabel("New password").fill(NEW_PASSWORD);
	await section.getByLabel("Confirm the password").fill(NEW_PASSWORD);
	await section.getByRole("button", { name: "Change password" }).click();
	await expect(section.getByRole("status")).toContainText("changed");
});

test("Settings makes a new recovery key and the old one stops working (VV-382)", async ({
	page,
}) => {
	await page.goto("/account/signin");
	await signIn(page, NEW_PASSWORD);
	await expect(page).toHaveURL(/\/$/);
	await page.goto("/settings");
	const form = page.getByTestId("settings-recovery-key");

	await form.getByLabel("Your password, to make the key").fill(PASSWORD);
	await form.getByRole("button", { name: "Make a new recovery key" }).click();
	await expect(page.locator("#settings-account-error")).toBeVisible();
	await expect(page.getByTestId("recovery-key")).toHaveCount(0);

	await form.getByLabel("Your password, to make the key").fill(NEW_PASSWORD);
	await form.getByRole("button", { name: "Make a new recovery key" }).click();
	const made = (await page.getByTestId("recovery-key").textContent()) ?? "";
	expect(made).toMatch(/^([0-9A-Z]{4}-){12}[0-9A-Z]{1,4}$/);
	expect(made).not.toBe(recoveryKey);
	replacedRecoveryKey = recoveryKey;
	recoveryKey = made;
});

test("Logout locks the studio: the pages and the API answer with the sign-in and E_LOCKED", async ({
	page,
	request,
}) => {
	await page.goto("/account/signin");
	await signIn(page, NEW_PASSWORD);
	await expect(page).toHaveURL(/\/$/);

	await page.getByTestId("account-logout").click();
	await expect(page).toHaveURL(/\/account\/signin$/);

	await page.goto("/projects");
	await expect(page).toHaveURL(/\/account\/signin$/);

	const token = (await readFile(join(accountHome, "token"), "utf8")).trim();
	const refused = await request.get("/api/v1/projects", {
		headers: { Authorization: `Bearer ${token}` },
	});
	expect(refused.status()).toBe(423);
	expect(((await refused.json()) as { code: string }).code).toBe("E_LOCKED");
});

test("a forgotten password is recovered with the recovery key", async ({
	page,
}) => {
	await page.goto("/account/signin");
	await page.getByRole("link", { name: "I forgot my password" }).click();
	await expect(page).toHaveURL(/\/account\/recover$/);

	await page.getByLabel("Recovery key").fill("AAAA-BBBB-CCCC");
	await page.getByLabel("New password").fill(PASSWORD);
	await page.getByLabel("Confirm the password").fill(PASSWORD);
	await page.getByRole("button", { name: "Set the new password" }).click();
	await expect(page.getByRole("alert")).toBeVisible();

	// The key of the creation was replaced in Settings and opens nothing any more.
	await page.getByLabel("Recovery key").fill(replacedRecoveryKey);
	await page.getByRole("button", { name: "Set the new password" }).click();
	await expect(page.getByRole("alert")).toBeVisible();

	await page.getByLabel("Recovery key").fill(recoveryKey.toLowerCase());
	await page.getByRole("button", { name: "Set the new password" }).click();
	await expect(page).toHaveURL(/\/$/);
	await expect(page.getByTestId("sidebar")).toBeVisible();

	// The data is as it was: the new password is the one that opens it from now on.
	await page.getByTestId("account-logout").click();
	await expect(page).toHaveURL(/\/account\/signin$/);
	await signIn(page, PASSWORD);
	await expect(page).toHaveURL(/\/$/);
});
