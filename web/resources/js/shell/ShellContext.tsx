import {
	createContext,
	type ReactNode,
	type RefObject,
	useCallback,
	useContext,
	useMemo,
	useRef,
	useState,
} from "react";

import type { ApprovalQueueState } from "../dashboard/approval/useApprovalQueue";
import type { ChatPanelState } from "../dashboard/chat/useChat";

// What the parts of the shell share: the chat the dock talks to (one read of the session and its events,
// so the scene bar, the bell and the panel can never disagree), whether the panel of the right dock is
// open and the button that opens it, which gets the focus back when the panel closes.
export interface ShellState {
	chat: ChatPanelState;
	// the approvals that wait, kept live by the queue stream
	approvals: ApprovalQueueState;
	chatOpen: boolean;
	setChatOpen(open: boolean): void;
	openChat(): void;
	chatButton: RefObject<HTMLButtonElement | null>;
}

const ShellContext = createContext<ShellState | null>(null);

const PROJECT_IN_URL = /prj_[0-9a-hjkmnp-tv-z]{10}/;

// The project a page is about, when its address has one: the chat talks about it.
export function projectInUrl(url: string): string | null {
	return PROJECT_IN_URL.exec(url)?.[0] ?? null;
}

export function ShellProvider({
	chat,
	approvals,
	children,
}: {
	chat: ChatPanelState;
	approvals: ApprovalQueueState;
	children: ReactNode;
}): ReactNode {
	const [chatOpen, setChatOpen] = useState(false);
	const chatButton = useRef<HTMLButtonElement | null>(null);
	const openChat = useCallback(() => setChatOpen(true), []);
	const value = useMemo(
		() => ({ chat, approvals, chatOpen, setChatOpen, openChat, chatButton }),
		[chat, approvals, chatOpen, openChat],
	);
	return (
		<ShellContext.Provider value={value}>{children}</ShellContext.Provider>
	);
}

export function useShell(): ShellState {
	const value = useContext(ShellContext);
	if (value === null) throw new Error("useShell needs the ShellProvider");
	return value;
}
