import { describe, expect, test } from "vitest";

import { parseInbox, waitingChatRequest, waitingCount } from "./notifications";
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
