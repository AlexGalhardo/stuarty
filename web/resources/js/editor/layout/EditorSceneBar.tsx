import {
	createContext,
	type ReactNode,
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useRef,
} from "react";
import { createPortal } from "react-dom";

import ChatDock from "@/dashboard/chat/ChatDock";
import { SessionProvider } from "@/lib/session";
import SceneBar from "../../shell/SceneBar";
import {
	LiveShellProvider,
	useShell,
	useShellSelector,
} from "../../shell/ShellContext";
import { projectNameOf, sceneOf } from "../../shell/sceneState";
import { usePageVisible } from "../../shell/usePageVisible";

// The chat panel over the editor (VV-427). It is drawn in the body, outside the stacking contexts of the editor
// regions, on the shell layer of the design system (z-40: over the editor, under the dialogs and menus), and it
// takes the right edge without moving anything of the editor. A key typed in it is the panel's: the shortcuts
// of the editor listen on the window, and the panel stops the keys before they get there.
function ChatOverlay(): ReactNode {
	const chatOpen = useShellSelector((s) => s.chatOpen);
	if (!chatOpen) return null;
	return createPortal(
		// biome-ignore lint/a11y/noStaticElementInteractions: the wrapper only keeps the keys of the panel from the shortcuts of the editor
		<div
			data-testid="editor-chat-layer"
			className="fixed inset-y-0 end-0 z-40 max-md:start-0"
			onKeyDown={(event) => event.stopPropagation()}
			onKeyUp={(event) => event.stopPropagation()}
		>
			<ChatDock />
		</div>,
		document.body,
	);
}

// Opens the chat from whichever button asked (the Chat button of the top bar or the one of the scene bar) and
// remembers it, so the focus goes back to it when the panel closes, as in the dashboard.
const OpenChat = createContext<(() => void) | null>(null);

export function useOpenEditorChat(): () => void {
	const open = useContext(OpenChat);
	if (open === null)
		throw new Error("useOpenEditorChat needs EditorShellFrame");
	return open;
}

function Frame({ children }: { children: ReactNode }): ReactNode {
	const chatOpen = useShellSelector((s) => s.chatOpen);
	const openChat = useShellSelector((s) => s.openChat);
	const opener = useRef<HTMLElement | null>(null);
	const was = useRef(false);
	useEffect(() => {
		if (was.current && !chatOpen && opener.current?.isConnected) {
			opener.current.focus();
		}
		was.current = chatOpen;
	}, [chatOpen]);
	const open = useCallback(() => {
		opener.current = document.activeElement as HTMLElement | null;
		openChat();
	}, [openChat]);
	return (
		<OpenChat.Provider value={open}>
			{children}
			<ChatOverlay />
		</OpenChat.Provider>
	);
}

// The session of the agent for this project, shared by the top bar (the bell and Chat), the scene bar under the
// timeline and the chat panel: one read, so they can never disagree. The streams are held only while the tab is
// on the screen, as on every dashboard page (usePageVisible).
export function EditorShellFrame({
	projectId,
	privileged,
	mode,
	children,
}: {
	projectId: string;
	privileged: boolean;
	mode: "local" | "network";
	children: ReactNode;
}): ReactNode {
	const visible = usePageVisible();
	const session = useMemo(() => ({ privileged, mode }), [privileged, mode]);
	return (
		<SessionProvider session={session}>
			<LiveShellProvider visible={visible} projectId={projectId}>
				<Frame>{children}</Frame>
			</LiveShellProvider>
		</SessionProvider>
	);
}

// The scene bar of the dashboard under the timeline of the editor (epic 85, SDD-012 section 23.4): the same bar,
// reading the same session of the agent for this project, so a permission is answered here without leaving. The
// bar reports what the service says; Allow and Deny send the call the chat panel sends.
export default function EditorSceneBar(): ReactNode {
	const { chat } = useShell();
	const open = useOpenEditorChat();
	return (
		<SceneBar
			model={sceneOf(chat.session, chat.events, chat.activity)}
			project={projectNameOf(chat.session, chat.projects)}
			connected={
				chat.profiles === null ? null : chat.profiles.profiles.length > 0
			}
			busy={chat.busy}
			onAnswer={(requestId, answer) => void chat.answer(requestId, answer)}
			onOpenChat={open}
		/>
	);
}
