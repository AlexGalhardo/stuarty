import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { WebhookDelivery } from "../../editor/api/types";
import { DeliveryRow, SecretNote } from "./Index";

vi.mock("@inertiajs/react", () => ({ Head: () => null, Link: () => null }));

const messages = {
	"ui.webhooks.state.failed": "Failed",
	"ui.webhooks.state.succeeded": "Delivered",
	"ui.webhooks.attempts": "attempt {n} of {max}",
	"ui.webhooks.status": "status {status}",
	"ui.webhooks.attempt_line": "Attempt {n}: status {status}, {ms} ms {error}",
	"ui.webhooks.redeliver": "Send again",
	"ui.webhooks.secret_title": "Signing secret of {name}",
	"ui.webhooks.secret_warning": "It is shown once.",
	"ui.webhooks.copy": "Copy",
	"ui.webhooks.copied": "Copied",
	"ui.webhooks.secret_saved": "I saved it",
};

const delivery = (extra: Partial<WebhookDelivery> = {}): WebhookDelivery => ({
	id: "wdl_1",
	webhook_id: "whk_1",
	event: "export.ready",
	event_id: "evt_1",
	kind: "event",
	state: "failed",
	attempts: 2,
	max_attempts: 6,
	last_status: 502,
	at: "2026-11-20T12:00:00Z",
	attempt_log: [
		{ n: 1, status: 502, duration_ms: 12, error: "status", response_bytes: 0 },
		{ n: 2, status: 502, duration_ms: 9, error: "status", response_bytes: 0 },
	],
	...extra,
});

function inI18n(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			{node}
		</I18nProvider>,
	);
}

describe("a delivery row", () => {
	test("shows its event, state, attempts, status and each attempt, with a button to send it again", () => {
		const html = inI18n(
			<ul>
				<DeliveryRow delivery={delivery()} onRedeliver={() => undefined} />
			</ul>,
		);
		expect(html).toContain("export.ready");
		expect(html).toContain("Failed");
		expect(html).toContain("attempt 2 of 6");
		expect(html).toContain("status 502");
		expect(html).toContain("Attempt 1: status 502, 12 ms status");
		expect(html).toContain("Send again");
		expect(html).toContain('data-delivery-state="failed"');
	});
});

describe("the signing secret note", () => {
	test("shows the secret once with a warning and a copy button", () => {
		const html = inI18n(
			<SecretNote
				revealed={{ id: "whk_1", name: "n8n", secret: "whsec_abc" }}
				onClose={() => undefined}
			/>,
		);
		expect(html).toContain("Signing secret of n8n");
		expect(html).toContain("whsec_abc");
		expect(html).toContain("It is shown once.");
		expect(html).toContain("Copy");
		expect(html).toContain("I saved it");
	});

	test("the name of a webhook reaches the page as text, never as markup", () => {
		const html = inI18n(
			<SecretNote
				revealed={{
					id: "whk_1",
					name: "<img src=x onerror=window.__pwned=1>",
					secret: "whsec_abc",
				}}
				onClose={() => undefined}
			/>,
		);
		expect(html).not.toContain("<img");
	});
});
