import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { ApprovalRequest } from "../../editor/api/types";
import ApprovalQueue from "./ApprovalQueue";

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.approvals.queue": "Waiting for you",
	"ui.approvals.live": "Live",
	"ui.approvals.offline": "Not live",
	"ui.approvals.empty": "Nothing is waiting for you.",
	"ui.approvals.kind": "Kind",
	"ui.approvals.destination": "Goes to",
	"ui.approvals.no_destination": "No destination",
	"ui.approvals.cost": "Estimated cost",
	"ui.approvals.no_cost": "Free",
	"ui.approvals.actor": "Asked by",
	"ui.approvals.asked": "Asked",
	"ui.approvals.reason": "Reason",
	"ui.approvals.approve": "Approve",
	"ui.approvals.reject": "Reject",
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

function markup(requests: ApprovalRequest[], live = true): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<ApprovalQueue
				requests={requests}
				destinations={{ con_1: "My blog" }}
				busyId={null}
				live={live}
				onDecide={() => undefined}
			/>
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

	test("a free action says it is free and one with no destination says so", () => {
		const html = markup([
			request({ estimate: { micros: 0, currency: "" }, connection_id: null }),
		]);
		expect(html).toContain("Free");
		expect(html).toContain("No destination");
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
