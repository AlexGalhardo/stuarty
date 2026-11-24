import type { ReactNode } from "react";

import type { ApprovalQueueState } from "@/dashboard/approval/useApprovalQueue";
import type { ChatPanelState } from "@/dashboard/chat/useChat";
import { I18nProvider } from "@/lib/i18n";
import { SessionProvider } from "@/lib/session";
import en from "../../lang/en.json";
import { ShellProvider } from "./ShellContext";

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

// A queue that has read nothing.
export function fakeApprovals(
	over: Partial<ApprovalQueueState> = {},
): ApprovalQueueState {
	return {
		requests: null,
		live: false,
		failure: null,
		busyId: null,
		decide: nothing,
		...over,
	};
}

export function Frame({
	children,
	chat = fakeChat(),
	approvals = fakeApprovals(),
	privileged = true,
}: {
	children: ReactNode;
	chat?: ChatPanelState;
	approvals?: ApprovalQueueState;
	privileged?: boolean;
}): ReactNode {
	return (
		<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
			<SessionProvider session={{ privileged, mode: "local" }}>
				<ShellProvider chat={chat} approvals={approvals}>
					{children}
				</ShellProvider>
			</SessionProvider>
		</I18nProvider>
	);
}
