import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import en from "../../../lang/en.json";
import type { ChatActivity, ChatEvent } from "../../editor/api/types";
import ChatEventView from "./ChatEventView";

function flat(
	tree: Record<string, unknown>,
	prefix: string,
	out: Record<string, string> = {},
): Record<string, string> {
	for (const [key, value] of Object.entries(tree)) {
		if (typeof value === "string") out[`${prefix}${key}`] = value;
		else flat(value as Record<string, unknown>, `${prefix}${key}.`, out);
	}
	return out;
}
const messages = flat(en.ui, "ui.");

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

function draw(event: ChatEvent, open = false, personOnly?: string): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
			<ol>
				<ChatEventView
					event={event}
					open={open}
					busy={false}
					personOnly={personOnly}
					onAnswer={() => {}}
				/>
			</ol>
		</I18nProvider>,
	);
}

const at = "2026-11-19T12:00:00Z";

describe("ChatEventView: a permission request in a session that is not a person's (VV-331)", () => {
	const request: ChatEvent = {
		seq: 9,
		kind: "permission_request",
		at,
		permission: { request_id: "r1", title: "Run a tool" },
	} as ChatEvent;
	const buttons = (html: string): string[] =>
		html.match(/<button[^>]*>/g) ?? [];

	test("its three answers are disabled and described by the reason", () => {
		const found = buttons(draw(request, true, "why"));
		expect(found).toHaveLength(3);
		for (const button of found) {
			expect(button).toContain('disabled=""');
			expect(button).toContain('aria-describedby="why"');
		}
	});

	test("a person can answer", () => {
		const found = buttons(draw(request, true));
		expect(found).toHaveLength(3);
		for (const button of found) {
			expect(button).not.toContain("disabled=");
			expect(button).not.toContain("aria-describedby");
		}
	});
});

describe("ChatEventView: what an agent writes is text", () => {
	test("a message with markup shows the markup, it does not make an element", () => {
		for (const kind of [
			"agent_message",
			"agent_thought",
			"user_message",
			"plan",
			"notice",
		] as const) {
			const html = draw({ seq: 1, kind, at, text: PAYLOAD });
			expect(html).toContain("&lt;img src=x onerror=window.__pwned=1&gt;");
			expect(html).not.toContain("<img");
		}
	});

	test("a tool call shows its title and its output as text", () => {
		const html = draw({
			seq: 2,
			kind: "tool_call",
			at,
			tool: {
				tool_call_id: "c",
				title: PAYLOAD,
				status: "completed",
				output: PAYLOAD,
			},
		});
		expect(html).not.toContain("<img");
		expect(html.match(/&lt;img/g)).toHaveLength(2);
		expect(html).toContain("done");
	});

	test("a link in the text of the agent is text in full, never an anchor", () => {
		const html = draw({
			seq: 3,
			kind: "agent_message",
			at,
			text: "see https://example.com/a?b=c for details",
		});
		expect(html).toContain("https://example.com/a?b=c");
		expect(html).not.toMatch(/<a [^>]*example\.com/);
	});
});

describe("ChatEventView: a permission request", () => {
	const ask: ChatEvent = {
		seq: 4,
		kind: "permission_request",
		at,
		permission: {
			request_id: "cpr_0000000001",
			title: PAYLOAD,
			options: [
				{ option_id: "a", name: "Always", kind: "allow_always" },
				{ option_id: "o", name: "Once", kind: "allow_once" },
			],
		},
	};

	test("offers one-time answers while it waits, and never a standing one", () => {
		const html = draw(ask, true);
		expect(html).toContain("Allow once");
		expect(html).toContain("Reject");
		expect(html).not.toContain("Always");
		expect(html).not.toContain("<img");
	});

	test("offers no answer once it was answered", () => {
		const html = draw(ask, false);
		expect(html).not.toContain("Allow once");
	});

	test("says that approvals are decided elsewhere, with a link to the page", () => {
		const html = draw(ask, true);
		expect(html).toContain(
			"Publications and costly generations are decided in Approvals, not here.",
		);
		expect(html).toContain('href="/approvals"');
	});
});

describe("ChatEventView: lines that are not shown", () => {
	test("a state event draws nothing", () => {
		expect(draw({ seq: 5, kind: "state", at, state: { state: "idle" } })).toBe(
			"<ol></ol>",
		);
	});
});

describe("ChatEventView: a skill the agent read", () => {
	test("says that the skill was loaded in this session, with its name as text", () => {
		const html = draw({
			seq: 6,
			kind: "skill_loaded",
			at,
			skill: { name: PAYLOAD, hash: "sha256:aa" },
		});
		expect(html).toContain('data-testid="chat-skill"');
		expect(html).toContain("Skill loaded in this session");
		expect(html).toContain("&lt;img src=x onerror=window.__pwned=1&gt;");
		expect(html).not.toContain("<img");
		expect(html).not.toMatch(/in use|following/i);
	});
});

describe("ChatEventView: the card of a request that waits shows what the scene bar shows", () => {
	const ask: ChatEvent = {
		seq: 7,
		kind: "permission_request",
		at,
		permission: { request_id: "r7", title: "Delete the clips" },
	};
	const activity = {
		session_id: "cht_1",
		state: "awaiting_permission",
		skill: { name: PAYLOAD, state: "loaded" },
		skills_loaded: [],
		tool: {
			title: PAYLOAD,
			technical_name: "stuarty_delete_clips",
			description_key: "ui.shell.scene.tool_from_agent",
			status: "pending",
			from_agent: false,
		},
		updated_at: at,
		untrusted_notice: "",
	} as unknown as ChatActivity;

	function drawWith(open: boolean): string {
		return renderToString(
			<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
				<ol>
					<ChatEventView
						event={ask}
						open={open}
						busy={false}
						agent={PAYLOAD}
						activity={activity}
						onAnswer={() => {}}
					/>
				</ol>
			</I18nProvider>,
		);
	}

	test("names the agent, the skill and its state, and the tool, as text", () => {
		const html = drawWith(true);
		expect(html).toContain('data-testid="chat-permission-agent"');
		expect(html).toContain('data-testid="chat-activity-skill"');
		expect(html).toContain("loaded in this session");
		expect(html).toContain('data-testid="chat-activity-tool"');
		expect(html).toContain("stuarty_delete_clips");
		expect(html).not.toContain("<img");
		expect(html.match(/&lt;img/g)?.length).toBeGreaterThanOrEqual(3);
	});

	test("says the bar and the center show the same request", () => {
		expect(drawWith(true)).toContain(
			"notification center show this same request",
		);
	});

	test("shows none of it once the request was answered", () => {
		const html = drawWith(false);
		expect(html).not.toContain("chat-activity");
		expect(html).not.toContain("chat-permission-agent");
	});
});
