import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { caller, expect, test } from "./helpers";

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

test.describe("connections", () => {
	test("add, test and remove a connection; the secret is never shown and a removed connection stops traffic", async ({
		page,
		request,
	}) => {
		const dest = await destination();
		try {
			await page.goto("/connections");
			await expect(
				page.getByRole("heading", { name: "Connections", level: 1 }),
			).toBeVisible();
			await expect(
				page.getByText("Nothing leaves this computer."),
			).toBeVisible();

			await page.getByLabel("Name", { exact: true }).fill("My blog");
			await page.getByLabel("Address (https)").fill(dest.url);
			await page.getByLabel("Credential").fill(SECRET);
			await page.getByRole("button", { name: "Connect" }).click();

			const row = page.locator("[data-connection-id]").first();
			await expect(row).toContainText("My blog");
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
			const api = await caller(request);
			expect(JSON.stringify(await api("GET", "/connections"))).not.toContain(
				SECRET,
			);

			await row.getByRole("button", { name: "Remove" }).click();
			await expect(page.locator("[data-connection-id]")).toHaveCount(0);
			await expect(
				page.getByText("Nothing leaves this computer."),
			).toBeVisible();
			expect(await api("GET", "/connections")).toEqual({ connections: [] });
			expect(dest.received).toHaveLength(1);
		} finally {
			await dest.close();
		}
	});

	test("the page is in Portuguese when the language is", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await page.goto("/connections");
		await expect(
			page.getByRole("heading", { name: "Conexões", level: 1 }),
		).toBeVisible();
		await expect(page.getByText("Nada sai deste computador.")).toBeVisible();
		await expect(page.getByRole("button", { name: "Conectar" })).toBeDisabled();
	});

	test("in Portuguese the page lists a connection with its destinations and its last use", async ({
		page,
		request,
	}) => {
		const dest = await destination();
		try {
			const api = await caller(request);
			await api("POST", "/connections", {
				kind: "test",
				label: "Meu blog",
				endpoint: dest.url,
				accepts: ["text", "video"],
				secret: SECRET,
			});

			await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
			await page.goto("/connections");
			const row = page.locator("[data-connection-id]").first();
			await expect(row).toContainText("Meu blog");
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
			await expect(page.locator("[data-connection-id]")).toHaveCount(0);
			await expect(page.getByText("Nada sai deste computador.")).toBeVisible();
		} finally {
			await dest.close();
		}
	});

	test("a destination that is not https is refused in words, and nothing is added", async ({
		page,
	}) => {
		await page.goto("/connections");
		await page.getByLabel("Name", { exact: true }).fill("Plain");
		await page.getByLabel("Address (https)").fill("http://example.com/x");
		await page.getByLabel("Credential").fill(SECRET);
		await page.getByRole("button", { name: "Connect" }).click();
		await expect(page.getByRole("alert")).toContainText("https");
		await expect(page.locator("[data-connection-id]")).toHaveCount(0);
	});
});
