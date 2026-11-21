import { createHmac } from "node:crypto";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { caller, expect, test } from "./helpers";

// Epic 59 (SDD-022): the Webhooks page registers an address and its events, shows the signing secret once, sends a
// test to a receiver on loopback, lists the delivery with its status and sends it again. The receiver of this spec
// checks the signature with the secret the page showed.
test.describe.configure({ mode: "serial" });

interface Received {
	signature: string;
	event: string;
	body: string;
}

interface Receiver {
	url: string;
	got: Received[];
	close(): Promise<void>;
}

async function receiver(): Promise<Receiver> {
	const got: Received[] = [];
	const server: Server = createServer((request, response) => {
		const chunks: Buffer[] = [];
		request.on("data", (c: Buffer) => chunks.push(c));
		request.on("end", () => {
			got.push({
				signature: String(request.headers["stuarty-signature"]),
				event: String(request.headers["stuarty-event"]),
				body: Buffer.concat(chunks).toString("utf8"),
			});
			response.statusCode = 204;
			response.end("ANSWER-CANARY");
		});
	});
	await new Promise<void>((resolve) =>
		server.listen(0, "127.0.0.1", () => resolve()),
	);
	const { port } = server.address() as AddressInfo;
	return {
		url: `http://127.0.0.1:${port}/hook?token=abc`,
		got,
		close: () => new Promise((resolve) => server.close(() => resolve())),
	};
}

function verifies(secret: string, received: Received): boolean {
	const parts = Object.fromEntries(
		received.signature.split(",").map((p) => p.split("=") as [string, string]),
	);
	const mac = createHmac("sha256", secret)
		.update(`${parts.t}.${received.body}`)
		.digest("hex");
	return mac === parts.v1;
}

let sequence = 0;

function uniqueName(name: string): string {
	sequence++;
	return `${name} ${Date.now().toString(36)}-${process.pid}-${sequence}`;
}

test.describe("webhooks", () => {
	test("register, see the secret once, send a test, see the delivery and send it again", async ({
		page,
		request,
	}) => {
		const target = await receiver();
		const name = uniqueName("n8n flow");
		const api = await caller(request);
		try {
			await page.goto("/webhooks");
			await expect(
				page.getByRole("heading", { name: "Webhooks", level: 1 }),
			).toBeVisible();
			await expect(
				page.getByText(
					"No webhook yet. Nothing leaves this computer until you register one.",
				),
			).toBeVisible();

			await page.getByLabel("Name", { exact: true }).fill(name);
			await page.getByLabel("Address", { exact: true }).fill(target.url);
			await page.getByLabel(/An export is ready/).check();
			await page.getByLabel(/A job failed/).check();
			await page.getByRole("button", { name: "Add webhook" }).click();

			const note = page.locator("[data-webhook-secret-note]");
			await expect(note).toContainText("Signing secret of");
			const secret = (await note
				.locator("[data-webhook-secret]")
				.textContent()) as string;
			expect(secret).toMatch(/^whsec_[A-Za-z0-9_-]{43}$/);

			const row = page.locator("[data-webhook-id]").filter({ hasText: name });
			await expect(row).toContainText("An export is ready, A job failed");
			await expect(row).toContainText("No delivery yet");

			await row.getByRole("button", { name: "Send test" }).click();
			await expect(row).toContainText("Last delivery: Delivered");
			expect(target.got).toHaveLength(1);
			expect(target.got[0].event).toBe("webhook.test");
			expect(verifies(secret, target.got[0])).toBe(true);
			expect(verifies(`${secret}x`, target.got[0])).toBe(false);

			await row.getByRole("button", { name: "Deliveries" }).click();
			const delivery = row.locator("[data-delivery-id]");
			await expect(delivery).toHaveCount(1);
			await expect(delivery).toContainText("webhook.test");
			await expect(delivery).toContainText("Delivered");
			await expect(delivery).toContainText("status 204");

			await delivery.getByRole("button", { name: "Send again" }).click();
			await expect(row.locator("[data-delivery-id]")).toHaveCount(2);
			await expect.poll(() => target.got.length).toBe(2);
			expect(verifies(secret, target.got[1])).toBe(true);

			// Rotating cannot be undone, so it asks first, and cancelling keeps the secret.
			await row.getByRole("button", { name: "Rotate secret" }).click();
			const ask = page.getByRole("alertdialog");
			await expect(ask).toContainText("refuses every call");
			await ask.getByRole("button", { name: "Cancel" }).click();
			await expect(ask).toHaveCount(0);
			await expect(note.locator("[data-webhook-secret]")).toHaveText(secret);

			// Confirmed, it shows a new secret, and the next call verifies with that one and not with the old.
			await row.getByRole("button", { name: "Rotate secret" }).click();
			await ask.getByRole("button", { name: "Rotate secret" }).click();
			await expect(note.locator("[data-webhook-secret]")).not.toHaveText(
				secret,
			);
			const rotated = (await note
				.locator("[data-webhook-secret]")
				.textContent()) as string;
			await row.getByRole("button", { name: "Send test" }).click();
			await expect.poll(() => target.got.length).toBe(3);
			expect(verifies(rotated, target.got[2])).toBe(true);
			expect(verifies(secret, target.got[2])).toBe(false);

			// Closing the note forgets the secret; no answer of the API holds it, and no answer body is kept.
			await note.getByRole("button", { name: "I saved it" }).click();
			await expect(note).toHaveCount(0);
			const list = JSON.stringify(await api("GET", "/webhooks"));
			const rows = (await api("GET", "/webhooks")) as {
				webhooks: { id: string; name: string }[];
			};
			const mine = rows.webhooks.find((w) => w.name === name);
			expect(mine).toBeTruthy();
			const deliveries = JSON.stringify(
				await api("GET", `/webhooks/${mine?.id}/deliveries`),
			);
			for (const shown of [secret, rotated]) {
				expect(list).not.toContain(shown);
				expect(deliveries).not.toContain(shown);
			}
			expect(deliveries).not.toContain("ANSWER-CANARY");

			// Removing asks first too: cancelling keeps the webhook.
			await row.getByRole("button", { name: "Remove" }).click();
			await ask.getByRole("button", { name: "Cancel" }).click();
			await expect(ask).toHaveCount(0);
			await expect(row).toHaveCount(1);

			await row.getByRole("button", { name: "Remove" }).click();
			await ask.getByRole("button", { name: "Remove" }).click();
			await expect(row).toHaveCount(0);
		} finally {
			const rows = (await api("GET", "/webhooks")) as {
				webhooks: { id: string; name: string }[];
			};
			for (const w of rows.webhooks.filter((x) => x.name === name)) {
				await api("DELETE", `/webhooks/${w.id}`);
			}
			await target.close();
		}
	});

	test("an address on a private network is refused with the reason", async ({
		page,
	}) => {
		await page.goto("/webhooks");
		await page.getByLabel("Name", { exact: true }).fill("internal");
		await page
			.getByLabel("Address", { exact: true })
			.fill("https://10.0.0.5/hook");
		await page.getByLabel(/A job failed/).check();
		await page.getByRole("button", { name: "Add webhook" }).click();
		await expect(page.getByRole("alert")).toContainText("private or internal");
		await expect(page.locator("[data-webhook-secret-note]")).toHaveCount(0);
	});

	test("the page is in Portuguese when the language is", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await page.goto("/webhooks");
		await expect(
			page.getByRole("heading", { name: "Webhooks de saída", level: 1 }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Adicionar webhook" }),
		).toBeDisabled();
	});
});
