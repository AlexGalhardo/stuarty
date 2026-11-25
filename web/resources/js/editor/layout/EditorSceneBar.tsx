import { type ReactNode, useEffect, useRef } from "react";
import { createPortal } from "react-dom";

import ChatDock from "@/dashboard/chat/ChatDock";
import { useChat } from "@/dashboard/chat/useChat";
import { SessionProvider } from "@/lib/session";
import SceneBar from "../../shell/SceneBar";
import { ShellProvider, useShell } from "../../shell/ShellContext";
import { projectNameOf, sceneOf } from "../../shell/sceneState";
import { useInbox } from "../../shell/useInbox";
import { usePageVisible } from "../../shell/usePageVisible";

// The chat panel over the editor (VV-427). It is drawn in the body, outside the stacking contexts of the editor
// regions, on the shell layer of the design system (z-40: over the editor, under the dialogs and menus), and it
// takes the right edge without moving anything of the editor. A key typed in it is the panel's: the shortcuts
// of the editor listen on the window, and the panel stops the keys before they get there.
function ChatOverlay(): ReactNode {
	const { chatOpen } = useShell();
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

function Bar(): ReactNode {
	const { chat, chatOpen, openChat } = useShell();
	// the button that opened the panel gets the focus back when it closes, as in the dashboard
	const opener = useRef<HTMLElement | null>(null);
	const was = useRef(false);
	useEffect(() => {
		if (was.current && !chatOpen && opener.current?.isConnected) {
			opener.current.focus();
		}
		was.current = chatOpen;
	}, [chatOpen]);
	return (
		<>
			<SceneBar
				model={sceneOf(chat.session, chat.events, chat.activity)}
				project={projectNameOf(chat.session, chat.projects)}
				connected={
					chat.profiles === null ? null : chat.profiles.profiles.length > 0
				}
				busy={chat.busy}
				onAnswer={(requestId, answer) => void chat.answer(requestId, answer)}
				onOpenChat={() => {
					opener.current = document.activeElement as HTMLElement | null;
					openChat();
				}}
			/>
			<ChatOverlay />
		</>
	);
}

// The scene bar of the dashboard under the timeline of the editor (epic 85, SDD-012 section 23.4): the same bar,
// reading the same session of the agent for this project, so a permission is answered here without leaving. The
// bar reports what the service says; Allow and Deny send the call the chat panel sends.
export default function EditorSceneBar({
	projectId,
	privileged,
	mode,
}: {
	projectId: string;
	privileged: boolean;
	mode: "local" | "network";
}): ReactNode {
	// The streams are held only while the tab is on the screen, as on every dashboard page (usePageVisible).
	const visible = usePageVisible();
	const chat = useChat(visible, projectId);
	const inbox = useInbox(visible);
	return (
		<SessionProvider session={{ privileged, mode }}>
			<ShellProvider chat={chat} inbox={inbox}>
				<Bar />
			</ShellProvider>
		</SessionProvider>
	);
}
