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

// What the parts of the shell share: whether the chat panel of the right dock is open, and the button
// that opens it, which gets the focus back when the panel closes. The top bar, the scene bar and the
// notification center all open the same panel.
export interface ShellState {
	chatOpen: boolean;
	setChatOpen(open: boolean): void;
	openChat(): void;
	chatButton: RefObject<HTMLButtonElement | null>;
}

const ShellContext = createContext<ShellState | null>(null);

export function ShellProvider({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	const [chatOpen, setChatOpen] = useState(false);
	const chatButton = useRef<HTMLButtonElement | null>(null);
	const openChat = useCallback(() => setChatOpen(true), []);
	const value = useMemo(
		() => ({ chatOpen, setChatOpen, openChat, chatButton }),
		[chatOpen, openChat],
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
