import { describe, expect, test } from "vitest";

import {
	failedPublicationNotices,
	parseInbox,
	splitNotifications,
	waitingChatRequest,
	waitingCount,
} from "./notifications";
import { inboxRow } from "./testSupport";

const waiting = inboxRow();
const approval = inboxRow({
	id: "approval:apr_aaaaaaaaaa",
	source: "approval",
	source_id: "apr_aaaaaaaaaa",
	session_id: "",
});
const answered = inboxRow({
	id: "chat:cpr_bbbbbbbbbb",
	source_id: "cpr_bbbbbbbbbb",
	state: "approved",
});

describe("waitingCount", () => {
	test("counts only the rows that wait, from either source", () => {
		expect(waitingCount([waiting, approval, answered])).toBe(2);
		expect(waitingCount([answered])).toBe(0);
		expect(waitingCount([])).toBe(0);
	});
});

describe("parseInbox", () => {
	test("reads the list the service sent", () => {
		const list = parseInbox(
			JSON.stringify({ rows: [waiting], waiting: 1, untrusted_notice: "n" }),
		);
		expect(list?.rows).toHaveLength(1);
		expect(list?.waiting).toBe(1);
	});

	test("drops a frame that is not a list", () => {
		expect(parseInbox("not json")).toBeNull();
		expect(parseInbox("null")).toBeNull();
		expect(parseInbox('"text"')).toBeNull();
		expect(parseInbox("{}")).toBeNull();
		expect(parseInbox('{"rows":"x"}')).toBeNull();
	});
});

describe("waitingChatRequest", () => {
	test("is the chat row of the session that waits, and never an approval or an answered row", () => {
		const rows = [approval, answered, waiting];
		expect(waitingChatRequest(rows, "cht_aaaaaaaaaa")).toBe(waiting);
		expect(waitingChatRequest(rows, "cht_other")).toBeNull();
		expect(waitingChatRequest([approval, answered], "cht_aaaaaaaaaa")).toBe(
			null,
		);
		expect(waitingChatRequest(null, "cht_aaaaaaaaaa")).toBeNull();
	});
});

describe("splitNotifications and failedPublicationNotices", () => {
	test("splits waiting items from settled history", () => {
		const groups = splitNotifications([waiting, approval, answered]);
		expect(groups.waiting).toEqual([waiting, approval]);
		expect(groups.history).toEqual([answered]);
	});

	test("maps only failed publication targets newest first", () => {
		const notices = failedPublicationNotices([
			{
				id: "pub_1",
				project_id: "prj_1",
				title: "Launch post",
				export: "",
				post: "pit_1",
				created_at: "2026-11-20T10:00:00Z",
				updated_at: "2026-11-20T12:00:00Z",
				targets: [
					{
						id: "tgt_ok",
						publication_id: "pub_1",
						connection_id: "con_1",
						network: "blog_wordpress",
						fields: {},
						state: "published",
						scheduled_at: "2026-11-20T11:00:00Z",
						zone: "UTC",
						late_window_minutes: 0,
						attempts: 1,
						updated_at: "2026-11-20T11:01:00Z",
					},
					{
						id: "tgt_old",
						publication_id: "pub_1",
						connection_id: "con_2",
						network: "youtube",
						fields: {},
						state: "failed",
						scheduled_at: "2026-11-19T11:00:00Z",
						zone: "UTC",
						late_window_minutes: 0,
						attempts: 1,
						error: {
							code: "E_PUBLISH",
							message: "Quota reached",
							hint: "Try tomorrow",
							retryable: false,
						},
						updated_at: "2026-11-19T11:05:00Z",
					},
					{
						id: "tgt_new",
						publication_id: "pub_1",
						connection_id: "con_3",
						network: "blog_wordpress",
						fields: { title: "Override title" },
						state: "failed",
						scheduled_at: "2026-11-20T11:00:00Z",
						zone: "UTC",
						late_window_minutes: 0,
						attempts: 1,
						error: {
							code: "E_PUBLISH",
							message: "HTTP 502",
							retryable: true,
						},
						updated_at: "2026-11-20T12:05:00Z",
					},
				],
			},
		]);
		expect(notices.map((n) => n.target_id)).toEqual(["tgt_new", "tgt_old"]);
		expect(notices[0]?.title).toBe("Override title");
		expect(notices[0]?.message).toBe("HTTP 502");
		expect(notices[1]?.hint).toBe("Try tomorrow");
	});
});
