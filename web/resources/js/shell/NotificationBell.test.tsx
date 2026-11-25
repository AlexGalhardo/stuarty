import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import NotificationBell, { CenterBody } from "./NotificationBell";
import type { Notification } from "./notifications";
import { Frame, fakeInbox, inboxRow } from "./testSupport";

const permission = inboxRow({
	tool: "Remove <b>12</b> silences",
	skill: "caption-style",
});

const approval = inboxRow({
	id: "approval:apr_aaaaaaaaaa",
	source: "approval",
	source_id: "apr_aaaaaaaaaa",
	session_id: "",
	agent: "agent:claude",
	tool: "publish.upload",
	summary: "Publish to Instagram",
	destination: "con_aaaaaaaaaa / prj_aaaaaaaaaa",
	cost: { micros: 1_500_000, currency: "USD" },
	undo: "cannot",
	project_id: "prj_aaaaaaaaaa",
});

function body(
	items: Notification[],
	over: {
		privileged?: boolean;
		projectName?: (id: string) => string | null;
		connectionName?: (id: string) => string | null;
	} = {},
): string {
	const privileged = over.privileged ?? true;
	return renderToString(
		<Frame privileged={privileged}>
			<CenterBody
				items={items}
				projectName={over.projectName}
				connectionName={over.connectionName}
				failure={null}
				busyId={null}
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

	test("the count is every row that waits, from either source, and a word for a reader", () => {
		const html = renderToString(
			<Frame
				inbox={fakeInbox({
					rows: [
						permission,
						approval,
						inboxRow({ id: "chat:cpr_z", state: "rejected" }),
					],
				})}
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

	test("a pending approval is listed with its cost and where, and its action leads to Approvals", () => {
		const html = body([approval], {
			connectionName: () => "Instagram da Marina",
			projectName: () => "Lancamento do curso",
		});
		expect(html).toContain('data-kind="approval"');
		expect(html).toContain("Publish this item to Instagram da Marina");
		expect(html).toMatch(
			/data-testid="notification-destination"[^>]*>[^<]*<span[^>]*>Instagram da Marina</,
		);
		expect(html).toMatch(/data-testid="notification-cost"[^>]*>\$1\.50</);
		expect(html).not.toMatch(/con_|prj_|publish\.upload/);
		expect(html).toMatch(
			/href="\/approvals"[^>]*data-testid="notification-review"|data-testid="notification-review"[^>]*href="\/approvals"/,
		);
		expect(html).toContain("Review in Approvals");
		expect(html).toMatch(
			/data-testid="notification-undo"[^>]*>Cannot be undone</,
		);
		expect(html).not.toContain("notification-allow");
	});

	test("a chat permission is answered with Allow once and Deny, never always", () => {
		const html = body([permission]);
		expect(html).toContain('data-testid="notification-allow"');
		expect(html).toContain('data-testid="notification-deny"');
		expect(html).toContain("Allow once");
		expect(html).not.toMatch(/always/i);
	});

	test("a permission of a tool outside Stuarty says Not reported for the item and the place, and Not verified for the undo", () => {
		const html = body([permission]);
		for (const id of ["notification-item", "notification-destination"]) {
			expect(html).toMatch(
				new RegExp(`data-testid="${id}"[^>]*>Not reported<`),
			);
		}
		expect(html).toMatch(/data-testid="notification-undo"[^>]*>Not verified</);
		expect(html).toMatch(/data-testid="notification-cost"[^>]*>No cost</);
		expect(html).toMatch(
			/data-testid="notification-skill"[^>]*>.*caption-style/,
		);
		expect(html).not.toContain("Can be undone");
	});

	test("a permission of a tool of Stuarty says what it does in plain words, beside the title the agent wrote", () => {
		const html = body([
			inboxRow({
				tool: "mcp__stuarty__edit",
				action_key: "ui.agent_activity.tool.edit",
				item: "ast_3c5v8n2w6d",
				destination: "prj_aaaaaaaaaa",
				undo: "not_verified",
			}),
		]);
		expect(html).toContain("Changes a project or an image.");
		expect(html).toContain("mcp__stuarty__edit");
		expect(html).toContain("ast_3c5v8n2w6d");
		// D-140: an edit the agent names is the agent's claim, so the undo is not verified
		expect(html).toMatch(/data-testid="notification-undo"[^>]*>Not verified</);
		expect(html).not.toContain("Can be undone");
		expect(html).toContain("as the agent described the call");
	});

	test("the undo line draws the word of the service and nothing else", () => {
		expect(body([inboxRow({ undo: "can" })])).toMatch(
			/data-testid="notification-undo"[^>]*>Can be undone</,
		);
		expect(body([inboxRow({ undo: "cannot" })])).toMatch(
			/data-testid="notification-undo"[^>]*>Cannot be undone</,
		);
		// a word the client does not know is never a yes
		const unknown = inboxRow({
			undo: "maybe" as unknown as Notification["undo"],
		});
		expect(body([unknown])).toMatch(
			/data-testid="notification-undo"[^>]*>Not verified</,
		);
	});

	test("the title of the request is text, never markup", () => {
		const html = body([permission]);
		expect(html).toContain("Remove &lt;b&gt;12&lt;/b&gt; silences");
		expect(html).not.toContain("<b>12</b>");
	});

	test("every row says its state in a word, the answer of a person included", () => {
		const html = body([
			inboxRow({ state: "rejected" }),
			inboxRow({ id: "chat:cpr_c", state: "approved" }),
			{ ...approval, id: "approval:apr_b", state: "expired" },
			{ ...approval, id: "approval:apr_c", state: "approved" },
		]);
		expect(html).toContain('data-state="denied"');
		expect(html).toContain("Denied");
		expect(html).toContain('data-state="allowed"');
		expect(html).toContain('data-state="expired"');
		expect(html).toContain('data-state="approved"');
		expect(html).not.toContain("notification-allow");
	});

	test("a permission row names the project of its session, as text", () => {
		const html = body([permission], {
			projectName: () => "<i>Launch</i>",
		});
		expect(html).toContain('data-testid="notification-project"');
		expect(html).toContain("&lt;i&gt;Launch&lt;/i&gt;");
	});

	test("a session that is not a person sees the actions disabled with the reason", () => {
		const html = body([permission], { privileged: false });
		expect(html).toMatch(
			/<button[^>]*disabled[^>]*data-testid="notification-allow"/,
		);
		expect(html).toContain('id="why"');
	});
});
