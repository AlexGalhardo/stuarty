import {
	createContext,
	type ReactNode,
	type RefObject,
	useContext,
	useEffect,
	useRef,
} from "react";
import { useStore } from "zustand";
import { createStore, type StoreApi } from "zustand/vanilla";

import { type ChatPanelState, useChat } from "../dashboard/chat/useChat";
import { type InboxState, useInbox } from "./useInbox";

// What the parts of the shell share: the chat the dock talks to (one read of the session and its events,
// so the scene bar, the bell and the panel can never disagree), whether the panel of the right dock is
// open and the button that opens it, which gets the focus back when the panel closes.
export interface ShellState {
	chat: ChatPanelState;
	// everything that waits for a person, from the chat and from the approvals, as one list the service builds
	inbox: InboxState;
	chatOpen: boolean;
	setChatOpen(open: boolean): void;
	openChat(): void;
	chatButton: RefObject<HTMLButtonElement | null>;
}

const ShellStoreContext = createContext<StoreApi<ShellState> | null>(null);

const PROJECT_IN_URL = /prj_[0-9a-hjkmnp-tv-z]{10}/;

// The project a page is about, when its address has one: the chat talks about it.
export function projectInUrl(url: string): string | null {
	return PROJECT_IN_URL.exec(url)?.[0] ?? null;
}

const EMPTY_CHAT: ChatPanelState = {
	profiles: null,
	projects: [],
	projectId: null,
	session: null,
	events: [],
	activity: null,
	live: false,
	busy: false,
	failure: null,
	selectProject: () => undefined,
	needProfiles: () => undefined,
	start: async () => undefined,
	send: async () => undefined,
	cancel: async () => undefined,
	answer: async () => undefined,
	exportMarkdown: async () => undefined,
	removeSession: async () => undefined,
	saveProfile: async () => false,
	removeProfile: async () => undefined,
};

const EMPTY_INBOX: InboxState = {
	rows: null,
	live: false,
	failure: null,
	busyId: null,
	answer: async () => undefined,
};

function makeShellStore(
	chatButton: RefObject<HTMLButtonElement | null>,
	initialChat: ChatPanelState,
	initialInbox: InboxState,
): StoreApi<ShellState> {
	return createStore<ShellState>((set) => ({
		chat: initialChat,
		inbox: initialInbox,
		chatOpen: false,
		setChatOpen: (open) => set({ chatOpen: open }),
		openChat: () => set({ chatOpen: true }),
		chatButton,
	}));
}

function ShellStreams({
	store,
	visible,
	projectId,
}: {
	store: StoreApi<ShellState>;
	visible: boolean;
	projectId: string | null;
}): null {
	const chat = useChat(visible, projectId);
	const inbox = useInbox(visible);
	useEffect(() => {
		store.setState({ chat, inbox });
	}, [store, chat, inbox]);
	return null;
}

export function LiveShellProvider({
	visible,
	projectId,
	children,
}: {
	visible: boolean;
	projectId: string | null;
	children: ReactNode;
}): ReactNode {
	const chatButton = useRef<HTMLButtonElement | null>(null);
	const storeRef = useRef<StoreApi<ShellState> | null>(null);
	if (storeRef.current === null) {
		storeRef.current = makeShellStore(chatButton, EMPTY_CHAT, EMPTY_INBOX);
	}
	return (
		<ShellStoreContext.Provider value={storeRef.current}>
			<ShellStreams
				store={storeRef.current}
				visible={visible}
				projectId={projectId}
			/>
			{children}
		</ShellStoreContext.Provider>
	);
}

export function ShellProvider({
	chat,
	inbox,
	children,
}: {
	chat: ChatPanelState;
	inbox: InboxState;
	children: ReactNode;
}): ReactNode {
	const chatButton = useRef<HTMLButtonElement | null>(null);
	const storeRef = useRef<StoreApi<ShellState> | null>(null);
	if (storeRef.current === null) {
		storeRef.current = makeShellStore(chatButton, chat, inbox);
	}
	useEffect(() => {
		storeRef.current?.setState({ chat, inbox });
	}, [chat, inbox]);
	return (
		<ShellStoreContext.Provider value={storeRef.current}>
			{children}
		</ShellStoreContext.Provider>
	);
}

export function useShellSelector<T>(selector: (state: ShellState) => T): T {
	const store = useContext(ShellStoreContext);
	if (store === null) throw new Error("useShell needs the ShellProvider");
	return useStore(store, selector);
}

// The shell when the page is inside it; null on a page drawn on its own (a test, a frame without the dock).
export function useOptionalShell(): ShellState | null {
	const store = useContext(ShellStoreContext);
	const fallbackRef = useRef<StoreApi<ShellState | null> | null>(null);
	if (fallbackRef.current === null) {
		fallbackRef.current = createStore<ShellState | null>(() => null);
	}
	return useStore(
		(store ?? fallbackRef.current) as StoreApi<ShellState | null>,
		(s) => s,
	);
}

export function useShell(): ShellState {
	return useShellSelector((s) => s);
}
