import type { ReactNode } from "react";

import SceneBar from "./SceneBar";
import { useShell } from "./ShellContext";
import { projectNameOf, sceneOf } from "./sceneState";

// The scene bar of the page: the session the shell reads, told as the service reports it. Allow and Deny
// send the answer the chat panel sends, through the same call (chat.sessions.answer).
export default function ConnectedSceneBar(): ReactNode {
	const { chat, inbox, openChat } = useShell();
	return (
		<SceneBar
			model={sceneOf(chat.session, chat.events, chat.activity, inbox.rows)}
			project={projectNameOf(chat.session, chat.projects)}
			connected={
				chat.profiles === null ? null : chat.profiles.profiles.length > 0
			}
			busy={chat.busy}
			onAnswer={(requestId, answer) => void chat.answer(requestId, answer)}
			onOpenChat={openChat}
		/>
	);
}
