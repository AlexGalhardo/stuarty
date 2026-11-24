import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import type { ChatActivity } from "../../editor/api/types";
import { reportedTool } from "../../shell/sceneState";

// What the agent is doing, as the service reports it (GET /chat/sessions/{id}/activity, SDD-017 section 8.1): the
// skill it read in this session and the tool it called last, with the plain description of a tool of Stuarty. It
// draws the answer and adds nothing; the names and titles are text from the agent or from the library.
export default function ActivityStrip({
	activity,
}: {
	activity: ChatActivity | null;
}): ReactNode {
	const t = useT();
	const tool = activity === null ? null : reportedTool(activity);
	const skill = activity?.skill ?? null;
	const none = (
		<span className="text-muted-foreground">
			{t("ui.shell.scene.not_reported")}
		</span>
	);
	return (
		<dl
			aria-label={t("ui.chat.activity")}
			data-testid="chat-activity"
			className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-lg border px-3 py-2 text-sm"
		>
			<dt className="text-muted-foreground">{t("ui.shell.scene.skill")}</dt>
			<dd className="min-w-0 break-words" data-testid="chat-activity-skill">
				{skill === null ? (
					none
				) : skill.state === "loaded" ? (
					<>
						<UntrustedText>{skill.name}</UntrustedText>
						<span className="text-muted-foreground">
							{` · ${t("ui.shell.scene.skill_state.loaded")}`}
						</span>
					</>
				) : (
					<span className="text-muted-foreground">
						{t(
							skill.state === "available"
								? "ui.shell.scene.skill_available"
								: "ui.shell.scene.skill_none",
						)}
					</span>
				)}
			</dd>
			<dt className="text-muted-foreground">{t("ui.shell.scene.tool")}</dt>
			<dd className="min-w-0 break-words" data-testid="chat-activity-tool">
				{activity === null ? (
					none
				) : tool === null ? (
					<span className="text-muted-foreground">
						{t("ui.shell.scene.no_tool")}
					</span>
				) : (
					<>
						{tool.descriptionKey !== null ? (
							t(tool.descriptionKey)
						) : (
							<UntrustedText>{tool.title}</UntrustedText>
						)}
						<span className="text-muted-foreground">
							{` · ${tool.technicalName ?? t("ui.shell.scene.tool_from_agent")} · ${t(`ui.chat.tool_status_${tool.status}`)}`}
						</span>
					</>
				)}
			</dd>
		</dl>
	);
}
