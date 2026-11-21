import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import type { Locator, Page } from "@playwright/test";
import { caller, expect, test } from "./helpers";

// The section that adds a connection; the Blog section has a Name field of its own.
function newConnection(page: Page): Locator {
	return page.locator('section[aria-labelledby="connections-new"]');
}

// RF-220 (epic 04, SDD-014): the Connections page lists every destination with what it may
// receive and when it was last used, tests one, and removes one with its credential. The
// destination is a server of this spec on loopback that records what it receives.
test.describe.configure({ mode: "serial" });

const SECRET = "sk-e2e-CANARY-5f3a9c";

interface Destination {
	url: string;
	received: string[];
	close(): Promise<void>;
}

async function destination(): Promise<Destination> {
	const received: string[] = [];
	const server: Server = createServer((request, response) => {
		received.push(String(request.headers.authorization));
		response.end("pong from the destination");
	});
	await new Promise<void>((resolve) =>
		server.listen(0, "127.0.0.1", () => resolve()),
	);
	const { port } = server.address() as AddressInfo;
	return {
		url: `http://127.0.0.1:${port}/health`,
		received,
		close: () => new Promise((resolve) => server.close(() => resolve())),
	};
}

// Other specs of the suite leave connections in the shared workspace and run in parallel with
// this one (VV-253), so each test looks only at the connection it made, found by a label no
// other spec uses, and removes it at the end.
let sequence = 0;

function uniqueLabel(name: string): string {
	sequence++;
	return `${name} ${Date.now().toString(36)}-${process.pid}-${sequence}`;
}

async function removeByLabel(
	api: Awaited<ReturnType<typeof caller>>,
	label: string,
): Promise<void> {
	const list = (await api("GET", "/connections")) as {
		connections: { id: string; label: string }[];
	};
	for (const connection of list.connections.filter((c) => c.label === label)) {
		await api("DELETE", `/connections/${connection.id}`);
	}
}

async function labelled(
	api: Awaited<ReturnType<typeof caller>>,
	label: string,
): Promise<number> {
	const list = (await api("GET", "/connections")) as {
		connections: { label: string }[];
	};
	return list.connections.filter((c) => c.label === label).length;
}

test.describe("connections", () => {
	test("add, test and remove a connection; the secret is never shown and a removed connection stops traffic", async ({
		page,
		request,
	}) => {
		const dest = await destination();
		const label = uniqueLabel("My blog");
		const api = await caller(request);
		try {
			await page.goto("/connections");
			await expect(
				page.getByRole("heading", { name: "Connections", level: 1 }),
			).toBeVisible();
			await expect(
				page.getByText(
					"Until you connect something, nothing leaves your computer.",
				),
			).toBeVisible();

			await newConnection(page).getByLabel("Name", { exact: true }).fill(label);
			await page.getByLabel("Address (https)").fill(dest.url);
			await page.getByLabel("Credential").fill(SECRET);
			await page.getByRole("button", { name: "Connect" }).click();

			const row = page
				.locator("[data-connection-id]")
				.filter({ hasText: label });
			await expect(row).toContainText(label);
			await expect(row).toContainText("Reaches: 127.0.0.1");
			await expect(row).toContainText("May receive: Text");
			await expect(row).toContainText("Never used");
			await expect(row).toContainText("Credential stored");
			await expect(page.locator("body")).not.toContainText(SECRET);
			await expect(page.getByLabel("Credential")).toHaveValue("");

			await row.getByRole("button", { name: "Test" }).click();
			await expect(row.locator("[data-test-result]")).toContainText(
				"The destination answered 200.",
			);
			await expect(row.locator("[data-test-result]")).toContainText(
				"pong from the destination",
			);
			await expect(row).toContainText("Last used");
			expect(dest.received).toEqual([`Bearer ${SECRET}`]);

			// No answer of the API carries the credential.
			expect(JSON.stringify(await api("GET", "/connections"))).not.toContain(
				SECRET,
			);

			await row.getByRole("button", { name: "Remove" }).click();
			await expect(row).toHaveCount(0);
			expect(await labelled(api, label)).toBe(0);
			expect(dest.received).toHaveLength(1);
		} finally {
			await removeByLabel(api, label);
			await dest.close();
		}
	});

	test("the page is in Portuguese when the language is", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await page.goto("/connections");
		await expect(
			page.getByRole("heading", { name: "Conexões", level: 1 }),
		).toBeVisible();
		await expect(
			page.getByText(
				"Enquanto você não conectar nada, nada sai do seu computador.",
			),
		).toBeVisible();
		await expect(page.getByRole("button", { name: "Conectar" })).toBeDisabled();
	});

	test("in Portuguese the page lists a connection with its destinations and its last use", async ({
		page,
		request,
	}) => {
		const dest = await destination();
		const label = uniqueLabel("Meu blog");
		const api = await caller(request);
		try {
			await api("POST", "/connections", {
				kind: "test",
				label,
				endpoint: dest.url,
				accepts: ["text", "video"],
				secret: SECRET,
			});

			await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
			await page.goto("/connections");
			const row = page
				.locator("[data-connection-id]")
				.filter({ hasText: label });
			await expect(row).toContainText(label);
			await expect(row).toContainText("Acessa: 127.0.0.1");
			await expect(row).toContainText("Pode receber: Texto, Vídeo");
			await expect(row).toContainText("Nunca usada");
			await expect(row).toContainText("Credencial guardada");

			await row.getByRole("button", { name: "Testar" }).click();
			await expect(row.locator("[data-test-result]")).toContainText(
				"O destino respondeu 200.",
			);
			await expect(row).toContainText("Usada pela última vez em");

			await row.getByRole("button", { name: "Remover" }).click();
			await expect(row).toHaveCount(0);
			expect(await labelled(api, label)).toBe(0);
		} finally {
			await removeByLabel(api, label);
			await dest.close();
		}
	});

	test("a destination that is not https is refused in words, and nothing is added", async ({
		page,
		request,
	}) => {
		const label = uniqueLabel("Plain");
		const api = await caller(request);
		await page.goto("/connections");
		await newConnection(page).getByLabel("Name", { exact: true }).fill(label);
		await page.getByLabel("Address (https)").fill("http://example.com/x");
		await page.getByLabel("Credential").fill(SECRET);
		await page.getByRole("button", { name: "Connect" }).click();
		await expect(page.getByRole("alert")).toContainText("https");
		await expect(
			page.locator("[data-connection-id]").filter({ hasText: label }),
		).toHaveCount(0);
		expect(await labelled(api, label)).toBe(0);
	});
});
