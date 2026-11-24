import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import type { ChatEvent, ChatSession } from "../editor/api/types";
import NotificationBell, { CenterBody } from "./NotificationBell";
import { type Notification, notificationsOf } from "./notifications";
import { Frame, fakeApprovals, fakeChat } from "./testSupport";

const session: ChatSession = {
	id: "chs_1",
	project_id: "prj_1",
	profile: "Claude Code",
	state: "awaiting_permission",
	title: "",
	created_at: "",
	updated_at: "",
	event_count: 0,
	live: true,
};

const asked: ChatEvent = {
	seq: 1,
	kind: "permission_request",
	at: "2026-01-01T10:00:00Z",
	permission: {
		request_id: "r1",
		title: "Remove <b>12</b> silences",
		options: [
			{ option_id: "a", name: "Always allow", kind: "allow_always" },
			{ option_id: "b", name: "Allow once", kind: "allow_once" },
		],
	},
};

const pending = {
	id: "apr_1",
	actor: "agent:claude",
	kind: "publish.post",
	summary: "Publish to Instagram",
	connection_id: null,
	project_id: null,
	job_id: null,
	payload_ref: "",
	estimate: { micros: 0, currency: "" },
	state: "pending",
	decided_by: null,
	rule_id: null,
	created_at: "2026-01-01T11:00:00Z",
	decided_at: null,
	expires_at: "",
} as never;

function body(items: Notification[], privileged = true): string {
	return renderToString(
		<Frame privileged={privileged}>
			<CenterBody
				items={items}
				failure={null}
				busy={false}
				personOnlyId="why"
				privileged={privileged}
				onAnswer={() => undefined}
				onOpenChat={() => undefined}
				onNavigate={() => undefined}
			/>
		</Frame>,
	);
}

describe("NotificationBell", () => {
	test("with nothing waiting the bell has no count and a plain name", () => {
		const html = renderToString(
			<Frame>
				<NotificationBell />
			</Frame>,
		);
		expect(html).toContain('data-testid="bell"');
		expect(html).toContain('aria-label="Notifications"');
		expect(html).not.toContain("bell-count");
	});

	test("the count is the open permissions plus the pending approvals, and a word for a reader", () => {
		const html = renderToString(
			<Frame
				chat={fakeChat({ session, events: [asked] })}
				approvals={fakeApprovals({ requests: [pending] })}
			>
				<NotificationBell />
			</Frame>,
		);
		expect(html).toContain('aria-label="Notifications, 2 waiting for you"');
		expect(html).toMatch(
			/aria-hidden="true"[^>]*data-testid="bell-count"[^>]*>2</,
		);
	});
});

describe("CenterBody", () => {
	test("says nothing waits, when nothing does", () => {
		expect(body([])).toContain("Nothing waits for you.");
	});

	test("a pending approval is listed and its action leads to Approvals", () => {
		const html = body(notificationsOf(null, [], [pending]));
		expect(html).toContain('data-kind="approval"');
		expect(html).toContain("Publish to Instagram");
		expect(html).toMatch(
			/href="\/approvals"[^>]*data-testid="notification-review"|data-testid="notification-review"[^>]*href="\/approvals"/,
		);
		expect(html).toContain("Review in Approvals");
	});

	test("a chat permission is answered with Allow once and Deny, never always", () => {
		const html = body(notificationsOf(session, [asked], null));
		expect(html).toContain('data-testid="notification-allow"');
		expect(html).toContain('data-testid="notification-deny"');
		expect(html).toContain("Allow once");
		expect(html).not.toMatch(/always/i);
	});

	test("the title of the request is text, never markup", () => {
		const html = body(notificationsOf(session, [asked], null));
		expect(html).toContain("Remove &lt;b&gt;12&lt;/b&gt; silences");
		expect(html).not.toContain("<b>12</b>");
	});

	test("every row says its state in a word", () => {
		const answered: ChatEvent[] = [
			asked,
			{
				seq: 2,
				kind: "permission_answer",
				at: "2026-01-01T10:05:00Z",
				permission: { request_id: "r1", answer: "reject" },
			},
		];
		const html = body(
			notificationsOf({ ...session, state: "idle" }, answered, null),
		);
		expect(html).toContain('data-state="denied"');
		expect(html).toContain("Denied");
		expect(html).not.toContain("notification-allow");
	});

	test("a permission row names the project of its session, as text", () => {
		const html = renderToString(
			<Frame>
				<CenterBody
					items={notificationsOf(session, [asked], null)}
					project="<i>Launch</i>"
					failure={null}
					busy={false}
					personOnlyId="why"
					privileged
					onAnswer={() => undefined}
					onOpenChat={() => undefined}
					onNavigate={() => undefined}
				/>
			</Frame>,
		);
		expect(html).toContain('data-testid="notification-project"');
		expect(html).toContain("&lt;i&gt;Launch&lt;/i&gt;");
	});

	test("a session that is not a person sees the actions disabled with the reason", () => {
		const html = body(notificationsOf(session, [asked], null), false);
		expect(html).toMatch(
			/<button[^>]*disabled[^>]*data-testid="notification-allow"/,
		);
		expect(html).toContain('id="why"');
	});
});
