import {
	type APIRequestContext,
	test as base,
	type Page,
} from "@playwright/test";
import { NETWORK_ADMIN, NETWORK_URL } from "../support/network";
import { expect, seedProject, test } from "./helpers";

// P2-UI-029 (SDD-012 sections 2.1 and 22.2, RF-005, RF-203): the trash of the
// projects page, and the pages of network mode. The e2e server runs in local
// mode, so what is asserted here is the trash and the 404 of the keys page.
test.describe("network-mode-pages", () => {
	test("a project goes to the trash and comes back", async ({
		page,
		request,
	}) => {
		const label = `E2E trash ${Date.now()}`;
		const seeded = await seedProject(request, { name: label, tracks: 0 });

		await page.goto("/projects");
		const openLink = page.getByRole("link", {
			name: new RegExp(`Open.*${label}`),
		});
		await expect(openLink).toBeVisible();
		await page
			.getByRole("button", {
				name: new RegExp(`Actions for project.*${label}`),
			})
			.click();
		await page.getByRole("menuitem", { name: "Delete" }).click();
		await page
			.getByRole("alertdialog")
			.getByRole("button", { name: "Move to trash" })
			.click();
		await expect(openLink).toHaveCount(0);
		const trashed = page.locator(`[data-trashed-id='${seeded.id}']`);
		await expect(trashed).toBeVisible();

		await trashed.getByRole("button", { name: /^Restore/ }).click();
		await expect(trashed).toHaveCount(0);
		await expect(openLink).toBeVisible();
	});

	test("/login and /settings/keys answer 404 in local mode", async ({
		page,
	}) => {
		// The page fixture holds a local session: the 404 is the route, not the login.
		for (const path of ["/settings/keys", "/login"]) {
			const response = await page.goto(path);
			expect(response?.status(), path).toBe(404);
		}
	});
});

// The login of the server answers five attempts a minute for one email, and the
// run has three browsers and several tests that log in. Each test therefore has
// a member of its own, made by the administrator, whose session this worker
// opens once.
let admin: Promise<APIRequestContext> | undefined;
async function administrator(
	newContext: (options: { baseURL: string }) => Promise<APIRequestContext>,
): Promise<APIRequestContext> {
	admin ??= (async () => {
		const context = await newContext({ baseURL: NETWORK_URL });
		// Each worker opens a session, and five attempts a minute is the limit of
		// one email: on E_RATE_LIMITED wait the seconds the answer names.
		for (let attempt = 0; ; attempt++) {
			const login = await context.post("/auth/login", {
				data: NETWORK_ADMIN,
				headers: { Origin: NETWORK_URL },
			});
			if (login.ok()) break;
			if (login.status() !== 429 || attempt === 12) {
				throw new Error(`admin login: ${login.status()}`);
			}
			await new Promise((resolve) => setTimeout(resolve, 5_000));
		}
		return context;
	})();
	return admin;
}

const plain = base.extend<{ member: { email: string; password: string } }>({
	member: async ({ playwright }, use, testInfo) => {
		const context = await administrator((o) =>
			playwright.request.newContext(o),
		);
		const member = {
			email: `member-${Date.now()}-${testInfo.workerIndex}-${testInfo.parallelIndex}-${testInfo.repeatEachIndex}-${Math.floor(Math.random() * 1e6)}@example.test`,
			password: "e2e-member-password-1",
		};
		const created = await context.post("/api/v1/users", {
			data: { ...member, role: "member" },
			headers: { Origin: NETWORK_URL },
		});
		if (!created.ok()) {
			throw new Error(
				`create member: ${created.status()} ${await created.text()}`,
			);
		}
		await use(member);
	},
});

// The pages of network mode run against the network server of the config, which
// starts with one administrator. A cookie session posts JSON from this origin,
// which is what the API accepts as ambient authority (SDD-007 section 3.6).
// A plain Playwright test: the page fixture of helpers.ts logs in with the
// token file of the local server, which this one does not have.
plain.describe("network-mode-pages: login, keys and logout", () => {
	plain.use({ baseURL: NETWORK_URL });

	async function login(
		page: Page,
		member: { email: string; password: string },
		next = "/projects",
	): Promise<void> {
		await page.goto(`/login?next=${encodeURIComponent(next)}`);
		await page.getByLabel("Email").fill(member.email);
		await page.getByLabel("Password").fill(member.password);
		await page.getByRole("button", { name: "Log in" }).click();
		await expect(page).toHaveURL(`${NETWORK_URL}${next}`);
	}

	plain(
		"a page without a session goes to the login, and the login goes back",
		async ({ page, member }) => {
			await page.goto("/projects");
			await expect(page).toHaveURL(`${NETWORK_URL}/login?next=%2Fprojects`);
			await page.getByLabel("Email").fill(member.email);
			await page.getByLabel("Password").fill("not the password");
			await page.getByRole("button", { name: "Log in" }).click();
			await expect(page.getByRole("alert")).toBeVisible();
			await expect(page).toHaveURL(new RegExp(`^${NETWORK_URL}/login`));

			await page.getByLabel("Password").fill(member.password);
			await page.getByRole("button", { name: "Log in" }).click();
			await expect(page).toHaveURL(`${NETWORK_URL}/projects`);
		},
	);

	plain(
		"a key is created, its secret is shown once, and it is revoked",
		async ({ page, member }) => {
			await login(page, member, "/settings/keys");
			const name = `e2e key ${Date.now()}`;
			await page.getByLabel("Name").fill(name);
			await page.getByRole("button", { name: "Create key" }).click();
			const secret = page.locator("[data-key-secret]");
			await expect(secret).toBeVisible();
			await expect(secret).toContainText("shown only once");
			const row = page.getByRole("listitem").filter({ hasText: name });
			await expect(row).toBeVisible();

			await page.reload();
			await expect(page.locator("[data-key-secret]")).toHaveCount(0);
			await expect(row).toBeVisible();

			await row.getByRole("button", { name: /Revoke/ }).click();
			await expect(row).toContainText("Revoked");
		},
	);

	plain(
		"an ended session on the keys page goes to the login, and POST /auth/logout ends the session",
		async ({ page, member }) => {
			await login(page, member, "/settings/keys");
			// The list has loaded: the session ends after that, not under the load.
			await expect(page.getByText("No keys yet.")).toBeVisible();
			await page.context().clearCookies();
			await page.getByLabel("Name").fill("after the session ended");
			// The click starts a navigation of the page (the redirect to the login), so
			// it must not wait for it.
			await page
				.getByRole("button", { name: "Create key" })
				.click({ noWaitAfter: true });
			await expect(page).toHaveURL(
				`${NETWORK_URL}/login?next=%2Fsettings%2Fkeys`,
			);

			await login(page, member, "/projects");
			// The Log out button shows when the page has the shared prop auth.user,
			// which the server does not send yet (see the test below); the call it
			// makes is the one checked here.
			const status = await page.evaluate(async () => {
				const response = await fetch("/auth/logout", {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: "{}",
				});
				return response.status;
			});
			expect(status).toBeLessThan(300);
			await page.goto("/settings/keys");
			await expect(page).toHaveURL(
				`${NETWORK_URL}/login?next=%2Fsettings%2Fkeys`,
			);
		},
	);

	plain.fixme(
		"the Log out button and the keys link show in network mode",
		() => {
			// AppLayout draws them when page.props.auth.user is set, and no route shares
			// that prop in network mode yet (inertia.Share "auth" is missing in
			// web/routes). Enable this once it does: log in, expect the link
			// "API keys" and the button "Log out", click it, expect /login.
		},
	);
});
