import type { ReactNode } from "react";

import type { ChatPanelState } from "@/dashboard/chat/useChat";
import { I18nProvider } from "@/lib/i18n";
import { SessionProvider } from "@/lib/session";
import en from "../../lang/en.json";
import type { ChatActivity, InboxRow } from "../editor/api/types";
import { ShellProvider } from "./ShellContext";
import type { InboxState } from "./useInbox";

// The catalogue the server sends is the flattened "ui." tree; the tests of the shell read the English one.
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

export const messages = flat(en.ui, "ui.");

const nothing = async (): Promise<void> => undefined;

// A chat that has read nothing: the tests change what they need.
export function fakeChat(over: Partial<ChatPanelState> = {}): ChatPanelState {
	return {
		profiles: null,
		projects: [],
		projectId: null,
		session: null,
		events: [],
		activity: null,
		live: false,
		failure: null,
		busy: false,
		selectProject: () => undefined,
		needProfiles: () => undefined,
		start: nothing,
		send: nothing,
		cancel: nothing,
		answer: nothing,
		exportMarkdown: nothing,
		removeSession: nothing,
		saveProfile: async () => true,
		removeProfile: nothing,
		...over,
	};
}

// What the service answers for the activity of a session: the tests change what they need.
export function activity(over: Partial<ChatActivity> = {}): ChatActivity {
	return {
		session_id: "chs_1",
		state: "running",
		skill: { name: "", state: "none" },
		skills_loaded: [],
		tool: null,
		updated_at: "",
		untrusted_notice: "",
		...over,
	};
}

// An inbox that has read nothing.
export function fakeInbox(over: Partial<InboxState> = {}): InboxState {
	return {
		rows: null,
		live: false,
		failure: null,
		busyId: null,
		answer: nothing,
		...over,
	};
}

// A row of the unified list as the service sends it: the tests change what they need.
export function inboxRow(over: Partial<InboxRow> = {}): InboxRow {
	return {
		id: "chat:cpr_aaaaaaaaaa",
		source: "chat",
		source_id: "cpr_aaaaaaaaaa",
		state: "waiting",
		agent: "Claude Code",
		skill: "",
		tool: "Run a command",
		summary: "",
		action_key: "",
		item: "",
		destination: "",
		cost: null,
		undoable: false,
		project_id: "prj_aaaaaaaaaa",
		session_id: "cht_aaaaaaaaaa",
		created_at: "2026-01-01T10:00:00Z",
		answered_at: null,
		...over,
	};
}

export function Frame({
	children,
	chat = fakeChat(),
	inbox = fakeInbox(),
	privileged = true,
}: {
	children: ReactNode;
	chat?: ChatPanelState;
	inbox?: InboxState;
	privileged?: boolean;
}): ReactNode {
	return (
		<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
			<SessionProvider session={{ privileged, mode: "local" }}>
				<ShellProvider chat={chat} inbox={inbox}>
					{children}
				</ShellProvider>
			</SessionProvider>
		</I18nProvider>
	);
}
