import { createRef } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { SessionProvider } from "@/lib/session";
import en from "../../../lang/en.json";
import type { ChatEvent, ChatSession } from "../../editor/api/types";
import ChatConversation from "./ChatConversation";
import type { ChatPanelState } from "./useChat";

vi.mock("@inertiajs/react", () => ({ Link: () => null }));

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

const at = "2026-11-21T12:00:00Z";

// A live session waiting for the person, with a request open: every control of the panel is on the page.
const session = {
	id: "cht_1",
	project_id: "prj_1",
	profile: "agent",
	state: "awaiting_permission",
	title: "A chat",
	live: true,
} as unknown as ChatSession;

const events = [
	{
		seq: 2,
		kind: "permission_request",
		at,
		permission: { request_id: "r1", title: "Run a tool" },
	},
] as unknown as ChatEvent[];

const chat = {
	profiles: {
		profiles: [{ name: "agent", command: "", args: [] }],
		suggestions: [],
		can_write: true,
		available: true,
	},
	projects: [{ id: "prj_1", name: "A project" }],
	projectId: "prj_1",
	session,
	events,
	live: true,
	failure: null,
	busy: false,
} as unknown as ChatPanelState;

function draw(privileged: boolean): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
			<SessionProvider session={{ privileged, mode: "local" }}>
				<ChatConversation
					chat={chat}
					composerRef={createRef<HTMLTextAreaElement>()}
				/>
			</SessionProvider>
		</I18nProvider>,
	);
}

function buttonOf(html: string, label: string): string {
	const found = (html.match(/<button[^>]*>[\s\S]*?<\/button>/g) ?? []).find(
		(button) => button.replace(/<[^>]*>/g, "").trim() === label,
	);
	if (found === undefined) throw new Error(`no button ${label}`);
	return found.slice(0, found.indexOf(">") + 1);
}

const refused = [
	en.ui.chat.new_session,
	en.ui.chat.stop,
	en.ui.chat.end_session,
	en.ui.chat.delete,
	en.ui.chat.allow_once,
	en.ui.chat.reject,
	en.ui.chat.cancel_request,
];

describe("the chat in a session that is not a person's (VV-331)", () => {
	test("every control the server refuses is disabled and described by the reason", () => {
		const html = draw(false);
		const reason = /id="([^"]*person-only)"/.exec(html)?.[1];
		expect(reason).toBeDefined();
		expect(html).toContain("Only a person in the Stuarty desktop app");
		for (const label of [...refused, en.ui.chat.send]) {
			const button = buttonOf(html, label);
			expect(button, label).toContain('disabled=""');
			expect(button, label).toContain(`aria-describedby="${reason}"`);
		}
		expect(html).toMatch(/<textarea[^>]*\sdisabled=""/);
	});

	test("reading it stays possible: the export is enabled", () => {
		expect(buttonOf(draw(false), en.ui.chat.export)).not.toContain(
			'disabled=""',
		);
	});

	test("a person has every control, and no reason", () => {
		const html = draw(true);
		for (const label of refused) {
			expect(buttonOf(html, label), label).not.toContain('disabled=""');
		}
		expect(html).not.toContain("person-only");
	});
});
