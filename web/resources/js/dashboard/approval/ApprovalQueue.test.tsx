import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { type SessionInfo, SessionProvider } from "@/lib/session";
import type { ApprovalRequest, InboxRow } from "../../editor/api/types";
import ApprovalQueue from "./ApprovalQueue";
import type { ApprovalContext } from "./useApprovalContext";

vi.mock("@inertiajs/react", () => ({
	Link: ({ children, href }: { children: string; href: string }) => (
		<a href={href}>{children}</a>
	),
}));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.approvals.queue": "Waiting for you",
	"ui.approvals.live": "Live",
	"ui.approvals.offline": "Not live",
	"ui.approvals.empty": "Nothing is waiting for you.",
	"ui.approvals.kind": "Kind",
	"ui.approvals.destination": "Goes to",
	"ui.approvals.not_reported": "Not reported",
	"ui.approvals.loading": "Reading what is waiting for you…",
	"ui.approvals.what": "What happens",
	"ui.approvals.item": "Item",
	"ui.approvals.undo": "Undo",
	"ui.approvals.undo_yes": "Can be undone",
	"ui.approvals.undo_no": "Cannot be undone",
	"ui.approvals.undo_not_verified": "Not verified",
	"ui.approvals.skill": "Skill",
	"ui.approvals.path": "Path to publication",
	"ui.approvals.open_publish": "Open in Publish",
	"ui.approvals.words": { one: "{count} word", other: "{count} words" },
	"ui.approvals.address": "Address:",
	"ui.publish.state.waiting_approval": "Waiting for approval",
	"ui.approvals.cost": "Estimated cost",
	"ui.approvals.no_cost": "Free",
	"ui.approvals.actor": "Requested by",
	"ui.approvals.count": { one: "{count} request", other: "{count} requests" },
	"ui.approvals.person_decides":
		"Only a person decides. The agent does not approve.",
	"ui.approvals.asked": "Asked",
	"ui.approvals.reason": "Reason",
	"ui.approvals.approve": "Approve",
	"ui.approvals.reject": "Reject",
	"ui.session.person_only.local": "Approve it in the Stuarty desktop app.",
	"ui.session.person_only.network": "Sign in with your password.",
};

const request = (extra: Partial<ApprovalRequest> = {}): ApprovalRequest => ({
	id: "apr_1",
	actor: "key:key_1",
	kind: "publish.post",
	summary: "Post the launch video",
	connection_id: "con_1",
	project_id: null,
	job_id: "job_1",
	payload_ref: "job:job_1",
	estimate: { micros: 2_500_000, currency: "USD" },
	state: "pending",
	decided_by: null,
	rule_id: null,
	created_at: "2026-11-19T12:00:00Z",
	decided_at: null,
	expires_at: "2026-11-26T12:00:00Z",
	...extra,
});

const person: SessionInfo = { privileged: true, mode: "local" };

function markup(
	requests: ApprovalRequest[],
	live = true,
	session: SessionInfo = person,
	context: Record<string, ApprovalContext> = {},
): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<SessionProvider session={session}>
				<ApprovalQueue
					requests={requests}
					context={context}
					destinations={{ con_1: "My blog" }}
					busyId={null}
					live={live}
					onDecide={() => undefined}
				/>
			</SessionProvider>
		</I18nProvider>,
	);
}

describe("the approval queue shows what will go out, where, what it costs and who asked", () => {
	test("shows the summary, the destination, the cost and the actor", () => {
		const html = markup([request()]);
		expect(html).toContain("Post the launch video");
		expect(html).toContain("My blog");
		expect(html).toContain("$2.50");
		expect(html).toContain("key:key_1");
		expect(html).toContain("Approve");
		expect(html).toContain("Reject");
	});

	test("each request is a card with its waiting chip and the line that only a person decides, under a heading with the count", () => {
		const html = markup([request(), request({ id: "apr_2" })]);
		expect(html.match(/data-testid="approval-row"/g)).toHaveLength(2);
		expect(html.match(/data-status="waiting"/g)).toHaveLength(2);
		expect(
			html.match(/Only a person decides\. The agent does not approve\./g),
		).toHaveLength(2);
		expect(html).toMatch(/data-testid="approval-count"[^>]*>2 requests</);
		expect(html).toContain("Requested by");
		expect(html.indexOf("Reject")).toBeLessThan(html.indexOf(">Approve"));
	});

	test("a free action says it is free and one with no destination says so", () => {
		const html = markup([
			request({ estimate: { micros: 0, currency: "" }, connection_id: null }),
		]);
		expect(html).toContain("Free");
		expect(html).toContain("Not reported");
	});

	// Epic 82: what the service did not report reads "Not reported", never a guess, and undo is the service's word.
	test("with no context the undo, the skill, the item and the path read Not reported", () => {
		const html = markup([request()]);
		for (const id of ["undo", "skill", "item", "path"]) {
			expect(html).toMatch(
				new RegExp(`data-testid="approval-${id}"[^>]*>Not reported<`),
			);
		}
	});

	const row = (extra: Partial<InboxRow> = {}): InboxRow => ({
		id: "approval:apr_1",
		source: "approval",
		source_id: "apr_1",
		state: "waiting",
		agent: "key:key_1",
		skill: "publish-blog",
		tool: "publish.post",
		summary: "Post the launch video",
		action_key: "",
		item: "",
		destination: "",
		cost: null,
		undo: "cannot",
		project_id: "",
		session_id: "",
		created_at: "2026-11-19T12:00:00Z",
		answered_at: null,
		...extra,
	});

	test("shows the undo word and the skill the service reported", () => {
		const html = markup([request()], true, person, {
			apr_1: { row: row() },
		});
		expect(html).toContain("Cannot be undone");
		expect(html).toContain("publish-blog");
		// a word the client does not know is never read as "can"
		const odd = markup([request()], true, person, {
			apr_1: { row: row({ undo: "maybe" as InboxRow["undo"] }) },
		});
		expect(odd).toContain("Not verified");
		expect(odd).not.toContain("Can be undone");
	});

	test("a text post shows its excerpt, words and address and no media", () => {
		const html = markup([request()], true, person, {
			apr_1: {
				row: row(),
				publication: {
					id: "pub_1",
					project_id: "prj_1",
					title: "Launch notes",
					export: "",
					post: "itm_1",
					targets: [],
					created_at: "2026-11-19T12:00:00Z",
					updated_at: "2026-11-19T12:00:00Z",
				},
				target: {
					id: "tgt_1",
					publication_id: "pub_1",
					connection_id: "con_1",
					network: "blog_hugo",
					fields: {},
					state: "waiting_approval",
					scheduled_at: null,
					zone: "UTC",
					late_window_minutes: 0,
					attempts: 0,
					updated_at: "2026-11-19T12:00:00Z",
				},
				post: { words: 1, text: "Hello readers", slug: "launch-notes" },
			},
		});
		expect(html).toContain("Launch notes");
		expect(html).toContain("Hello readers");
		expect(html).toContain("1 word");
		expect(html).toContain("launch-notes");
		expect(html).toContain("Waiting for approval");
		expect(html).not.toMatch(/<(img|video|audio)\b/);
	});

	test("Approve and Reject are the same size", () => {
		const html = markup([request()]);
		const found = html.match(/<button[^>]*>/g) ?? [];
		expect(found.filter((b) => b.includes("min-w-24"))).toHaveLength(2);
	});

	// Epic 82: a queue that has not been read is not an empty one, and a failed read never says "nothing waits".
	test("loading and failed reads do not claim the queue is empty", () => {
		const read = (loading: boolean, failed: boolean): string =>
			renderToString(
				<I18nProvider messages={messages} locale="en" locales={["en"]}>
					<SessionProvider session={person}>
						<ApprovalQueue
							requests={[]}
							destinations={{}}
							loading={loading}
							failed={failed}
							busyId={null}
							live={false}
							onDecide={() => undefined}
						/>
					</SessionProvider>
				</I18nProvider>,
			);
		const loadingHtml = read(true, false);
		expect(loadingHtml).toContain("Reading what is waiting");
		expect(loadingHtml).not.toContain("Nothing is waiting for you.");
		const failedHtml = read(true, true);
		expect(failedHtml).not.toContain("Nothing is waiting for you.");
		expect(failedHtml).not.toContain("Reading what is waiting");
	});

	test("an empty queue says nothing waits, and the live state is shown", () => {
		expect(markup([])).toContain("Nothing is waiting for you.");
		expect(markup([], true)).toContain('data-live="true"');
		expect(markup([], false)).toContain('data-live="false"');
	});

	// SDD-007 section 8.5: the summary and the actor are text an agent wrote; they reach the page as
	// text, never as markup.
	test("text from an agent is escaped", () => {
		const html = markup([request({ summary: PAYLOAD, actor: PAYLOAD })]);
		expect(html).not.toContain(PAYLOAD);
		expect(html).toContain("&lt;img src=x onerror=window.__pwned=1&gt;");
	});
});

// VV-319: a session that is not a person's (stuarty open) sees the controls disabled, with the reason and where
// to do it; a person's session sees them enabled and no note.
// The attribute, not the Tailwind variants (`disabled:opacity-50`) in the class.
const DISABLED = /\sdisabled(=|\s|>)/;

describe("the answers of a person are offered only to a session that may give them", () => {
	const buttons = (html: string): string[] =>
		html.match(/<button[^>]*>/g)?.filter((b) => !b.includes("sr-only")) ?? [];

	test("an unprivileged session: Approve and Reject are disabled and the reason is shown", () => {
		const html = markup([request()], true, {
			privileged: false,
			mode: "local",
		});
		const found = buttons(html);
		expect(found).toHaveLength(2);
		for (const button of found) {
			expect(button).toMatch(DISABLED);
			expect(button).toContain('aria-describedby="approvals-person-only"');
		}
		expect(html).toContain('id="approvals-person-only"');
		expect(html).toContain("Approve it in the Stuarty desktop app.");
	});

	test("in network mode the reason says to sign in with the password", () => {
		const html = markup([request()], true, {
			privileged: false,
			mode: "network",
		});
		expect(html).toContain("Sign in with your password.");
	});

	test("a privileged session: the buttons are enabled and there is no note", () => {
		const html = markup([request()]);
		for (const button of buttons(html)) expect(button).not.toMatch(DISABLED);
		expect(html).not.toContain("approvals-person-only");
		expect(html).not.toContain("desktop app");
	});
});
