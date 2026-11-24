import type { ReactNode } from "react";

import { useApprovalQueue } from "@/dashboard/approval/useApprovalQueue";
import { useChat } from "@/dashboard/chat/useChat";
import { SessionProvider } from "@/lib/session";
import SceneBar from "../../shell/SceneBar";
import { ShellProvider, useShell } from "../../shell/ShellContext";
import { projectNameOf, sceneOf } from "../../shell/sceneState";
import { usePageVisible } from "../../shell/usePageVisible";

function Bar({ projectId }: { projectId: string }): ReactNode {
	const { chat } = useShell();
	return (
		<SceneBar
			model={sceneOf(chat.session, chat.events, chat.activity)}
			project={projectNameOf(chat.session, chat.projects)}
			connected={
				chat.profiles === null ? null : chat.profiles.profiles.length > 0
			}
			busy={chat.busy}
			onAnswer={(requestId, answer) => void chat.answer(requestId, answer)}
			// There is no chat dock in the editor: the project page has it.
			onOpenChat={() => location.assign(`/projects/${projectId}`)}
		/>
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
	const approvals = useApprovalQueue(visible);
	return (
		<SessionProvider session={{ privileged, mode }}>
			<ShellProvider chat={chat} approvals={approvals}>
				<Bar projectId={projectId} />
			</ShellProvider>
		</SessionProvider>
	);
}
