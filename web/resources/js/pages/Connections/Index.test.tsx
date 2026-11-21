import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { SessionProvider } from "@/lib/session";
import type { Connection, ConnectionTestResult } from "../../editor/api/types";
import { ConnectionRow } from "./Index";

vi.mock("@inertiajs/react", () => ({ Head: () => null, Link: () => null }));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.session.person_only.local": "Only a person in the app can do this.",
	"ui.connections.destinations": "Reaches: {hosts}",
	"ui.connections.accepts": "May receive: {list}",
	"ui.connections.accept.text": "Text",
	"ui.connections.accept.video": "Video",
	"ui.connections.never_used": "Never used",
	"ui.connections.last_used": "Last used {date}",
	"ui.connections.secret_set": "Credential stored",
	"ui.connections.secret_missing": "No credential",
	"ui.connections.test": "Test",
	"ui.connections.remove": "Remove",
	"ui.connections.test_ok": "Answered {status}.",
	"ui.connections.test_failed": "Failed ({status}).",
};

const connection = (extra: Partial<Connection> = {}): Connection => ({
	id: "con_1",
	kind: "test",
	label: PAYLOAD,
	endpoint: "https://blog.example.com",
	hosts: ["blog.example.com", "cdn.example.com"],
	accepts: ["text", "video"],
	created_at: "2026-11-19T12:00:00Z",
	last_used_at: null,
	secret_set: true,
	...extra,
});

function markup(
	c: Connection,
	result: ConnectionTestResult | null = null,
): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<ul>
				<ConnectionRow
					connection={c}
					result={result}
					onTest={() => undefined}
					onRemove={() => undefined}
				/>
			</ul>
		</I18nProvider>,
	);
}

// SDD-007 section 8.5, threat T-22: the label of a connection is text a person typed and the
// answer of a destination is text from the network; both reach the page as text, never as markup.
describe("connections page renders text from people and from the network as text", () => {
	test("the label", () => {
		const html = markup(connection());
		expect(html).toContain("&lt;img src=x onerror=window.__pwned=1&gt;");
		expect(html).not.toContain("<img");
	});

	test("the answer of a destination", () => {
		const html = markup(connection(), {
			id: "con_1",
			ok: false,
			status: 500,
			message: PAYLOAD,
		});
		expect(html).toContain("Failed (500).");
		expect(html).not.toContain("<img");
	});
});

describe("a connection row", () => {
	test("lists its destinations, what it accepts, its last use and that a credential is stored", () => {
		const html = markup(connection({ last_used_at: "2026-11-19T13:00:00Z" }));
		expect(html).toContain("Reaches: blog.example.com, cdn.example.com");
		expect(html).toContain("May receive: Text, Video");
		expect(html).toContain("Last used");
		expect(html).toContain("Credential stored");
		expect(html).not.toContain("Never used");
	});

	test("says so when it was never used or has no credential", () => {
		const html = markup(connection({ secret_set: false }));
		expect(html).toContain("Never used");
		expect(html).toContain("No credential");
	});

	const row = (signedIn: boolean, privileged: boolean): string =>
		renderToString(
			<I18nProvider messages={messages} locale="en" locales={["en"]}>
				<SessionProvider session={{ privileged, mode: "local" }}>
					<ul>
						<ConnectionRow
							connection={connection()}
							result={null}
							signedIn={signedIn}
							onTest={() => undefined}
							onRemove={() => undefined}
						/>
					</ul>
				</SessionProvider>
			</I18nProvider>,
		);
	const removeButton = (html: string): string =>
		(html.match(/<button[^>]*>/g) ?? [])[1] ?? "";

	test("a sign-in in a session that is not a person's cannot be removed, and says why (VV-331)", () => {
		const html = row(true, false);
		expect(removeButton(html)).toContain('disabled=""');
		expect(removeButton(html)).toContain(
			'aria-describedby="remove-person-only-con_1"',
		);
		expect(html).toContain('id="remove-person-only-con_1"');
		expect(html).toContain("Only a person");
	});

	test("a person removes a sign-in, and anyone removes a connection with a key", () => {
		for (const html of [row(true, true), row(false, false)]) {
			expect(removeButton(html)).not.toContain('disabled=""');
			expect(html).not.toContain("remove-person-only");
		}
	});
});
