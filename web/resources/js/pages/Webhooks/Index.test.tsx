import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { WebhookDelivery } from "../../editor/api/types";
import { DeliveryRow, NamedTitle, SecretNote } from "./Index";

vi.mock("@inertiajs/react", () => ({ Head: () => null, Link: () => null }));

const messages = {
	"ui.webhooks.state.failed": "Failed",
	"ui.webhooks.state.succeeded": "Delivered",
	"ui.webhooks.attempts": "attempt {n} of {max}",
	"ui.webhooks.status": "status {status}",
	"ui.webhooks.attempt_line": "Attempt {n}: status {status}, {ms} ms {error}",
	"ui.webhooks.redeliver": "Send again",
	"ui.webhooks.secret_title": "Signing secret of {name}",
	"ui.webhooks.confirm_remove_title": "Remove {name}?",
	"ui.webhooks.confirm_rotate_title": "Rotate the secret of {name}?",
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

// A right-to-left override inside a name: written as code points so that the file holds none.
const hostileName = `evil${String.fromCodePoint(0x202e)}fdp`;

// The title with the name in an element that isolates its direction, then `after`.
const isolated = (before: string, after: string): RegExp =>
	new RegExp(
		`${before}<span[^>]*unicode-bidi:isolate[^>]*>${hostileName}</span>${after}`,
	);

describe("the title that repeats the name of a webhook", () => {
	test("isolates the name in the question that confirms a removal or a rotation", () => {
		const html = inI18n(
			<>
				<NamedTitle
					messageKey="ui.webhooks.confirm_remove_title"
					name={hostileName}
				/>
				<NamedTitle
					messageKey="ui.webhooks.confirm_rotate_title"
					name={hostileName}
				/>
			</>,
		);
		expect(html).toMatch(isolated("Remove ", "\\?"));
		expect(html).toMatch(isolated("Rotate the secret of ", "\\?"));
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
		expect(html).toContain("Signing secret of ");
		expect(html).toContain(">n8n</span>");
		expect(html).toContain("whsec_abc");
		expect(html).toContain("It is shown once.");
		expect(html).toContain("Copy");
		expect(html).toContain("I saved it");
	});

	test("a name with bidirectional controls is isolated inside the title (SDD-007 section 8.5 item 3)", () => {
		const html = inI18n(
			<SecretNote
				revealed={{ id: "whk_1", name: hostileName, secret: "whsec_abc" }}
				onClose={() => undefined}
			/>,
		);
		expect(html).toMatch(isolated("Signing secret of ", ""));
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
