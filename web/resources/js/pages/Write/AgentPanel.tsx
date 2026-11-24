import { type ReactNode, useCallback, useEffect, useState } from "react";

import { Notice } from "@/components/Notice";
import { Button } from "@/components/ui/button";
import ActivityStrip from "@/dashboard/chat/ActivityStrip";
import { useT } from "@/lib/i18n";
import type { ChatActivity } from "../../editor/api/types";
import type { FlowStep, ProjectFlow, TextItem, WritingClient } from "./client";
import DraftPanel from "./DraftPanel";
import { isTextOnly } from "./meta";
import Panel from "./Panel";

type Reading =
	| { state: "loading" }
	| { state: "failed" }
	| { state: "ready"; activity: ChatActivity | null };

// What the agent may do on this text and what it is doing (the standing rule of the product: the skill, whether it
// is loaded, the tool, the path to the publication and the approval of anything that costs or publishes). Every
// word of the state is the service's; the page only draws it. In a project that holds only text the panel says in a
// sentence that cut, video caption and export do not apply, and offers the text actions alone (D-134).
export default function AgentPanel({
	client,
	item,
	flow,
	onDone,
}: {
	client: WritingClient;
	item: TextItem;
	flow: ProjectFlow | null;
	onDone: () => void;
}): ReactNode {
	const t = useT();
	const [reading, setReading] = useState<Reading>({ state: "loading" });
	const [again, setAgain] = useState(0);
	const projectId = item.project_id;
	const revision = item.revision;

	// biome-ignore lint/correctness/useExhaustiveDependencies: a new revision and the button are the reasons to read again
	useEffect(() => {
		let live = true;
		client
			.activity(projectId)
			.then((activity) => {
				if (live) setReading({ state: "ready", activity });
			})
			.catch(() => {
				if (live) setReading({ state: "failed" });
			});
		return () => {
			live = false;
		};
	}, [client, projectId, revision, again]);

	const readAgain = useCallback((): void => {
		setReading({ state: "loading" });
		setAgain((n) => n + 1);
	}, []);

	const step = flow?.steps.find((s) => s.id === flow.current);

	return (
		<Panel
			title={t("ui.write.agent.heading")}
			description={t("ui.write.agent.intro")}
			testId="agent-panel"
			headingId="agent-heading"
		>
			{isTextOnly(flow) && (
				<Notice
					tone="info"
					title={t("ui.write.agent.text_only")}
					data-testid="agent-text-only"
				>
					{t("ui.write.agent.text_only_body")}
				</Notice>
			)}
			<div className="flex flex-col gap-2" data-testid="agent-context">
				<ActivityStrip
					activity={reading.state === "ready" ? reading.activity : null}
				/>
				{reading.state === "loading" && (
					<p role="status" className="text-sm text-text-muted">
						{t("ui.write.agent.reading")}
					</p>
				)}
				{reading.state === "ready" && reading.activity === null && (
					<p className="text-sm text-text-muted">
						{t("ui.write.agent.no_session")}
					</p>
				)}
				{reading.state === "failed" && (
					<div className="flex flex-wrap items-center gap-2">
						<p role="alert" className="text-sm text-danger-text">
							{t("ui.write.agent.failed")}
						</p>
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={readAgain}
						>
							{t("ui.write.agent.retry")}
						</Button>
					</div>
				)}
				{reading.state === "ready" && (
					<div>
						<Button type="button" variant="ghost" size="sm" onClick={readAgain}>
							{t("ui.write.agent.refresh")}
						</Button>
					</div>
				)}
				<StepLine step={step} />
			</div>
			<DraftPanel client={client} item={item} onDone={onDone} />
		</Panel>
	);
}

// The step the service says the project is at, with who does it and the operation that does it.
function StepLine({ step }: { step: FlowStep | undefined }): ReactNode {
	const t = useT();
	if (step === undefined) return null;
	return (
		<p className="text-sm text-text-muted" data-testid="agent-step">
			{t("ui.write.agent.step", {
				step: t(`ui.write.flow.step.${step.id}`),
				actor: t(`ui.write.flow.actor.${step.actor}`),
			})}
			{step.tool !== undefined &&
				step.tool !== "" &&
				` · ${t("ui.write.agent.step_tool", { tool: step.tool })}`}
		</p>
	);
}
