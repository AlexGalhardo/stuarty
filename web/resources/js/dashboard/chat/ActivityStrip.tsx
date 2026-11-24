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
	testId = "chat-activity",
}: {
	activity: ChatActivity | null;
	// the card of a permission request holds a second strip: its own ids keep the two apart
	testId?: string;
}): ReactNode {
	const t = useT();
	const tool = activity === null ? null : reportedTool(activity);
	const skill = activity?.skill ?? null;
	const none = (
		<span className="text-text-muted">{t("ui.shell.scene.not_reported")}</span>
	);
	return (
		<dl
			aria-label={t("ui.chat.activity")}
			data-testid={testId}
			className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-lg border border-agent-border bg-agent-soft/40 px-3 py-2 text-sm"
		>
			<dt className="text-text-muted">{t("ui.shell.scene.skill")}</dt>
			<dd className="min-w-0 break-words" data-testid={`${testId}-skill`}>
				{skill === null ? (
					none
				) : skill.state === "loaded" ? (
					<>
						<UntrustedText>{skill.name}</UntrustedText>
						<span className="text-text-muted">
							{` · ${t("ui.shell.scene.skill_state.loaded")}`}
						</span>
					</>
				) : (
					<span className="text-text-muted">
						{t(
							skill.state === "available"
								? "ui.shell.scene.skill_available"
								: "ui.shell.scene.skill_none",
						)}
					</span>
				)}
			</dd>
			<dt className="text-text-muted">{t("ui.shell.scene.tool")}</dt>
			<dd className="min-w-0 break-words" data-testid={`${testId}-tool`}>
				{activity === null ? (
					none
				) : tool === null ? (
					<span className="text-text-muted">{t("ui.shell.scene.no_tool")}</span>
				) : (
					<>
						{tool.descriptionKey !== null ? (
							t(tool.descriptionKey)
						) : (
							<UntrustedText>{tool.title}</UntrustedText>
						)}
						<span className="text-text-muted">
							{` · ${tool.technicalName ?? t("ui.shell.scene.tool_from_agent")} · ${t(`ui.chat.tool_status_${tool.status}`)}`}
						</span>
						{tool.descriptionKey !== null && (
							// the tool is recognised from the title the agent gave the call, which the person sees as written
							<span className="block text-text-muted">
								{`${t("ui.shell.scene.tool_from_agent")}: `}
								<UntrustedText>{tool.title}</UntrustedText>
							</span>
						)}
					</>
				)}
			</dd>
		</dl>
	);
}
