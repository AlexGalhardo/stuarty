import { Head } from "@inertiajs/react";
import { SlidersHorizontal } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import ApprovalQueue from "../../dashboard/approval/ApprovalQueue";
import CostPanel from "../../dashboard/approval/CostPanel";
import RulesPanel from "../../dashboard/approval/RulesPanel";
import { useApprovalContext } from "../../dashboard/approval/useApprovalContext";
import { useApprovalQueue } from "../../dashboard/approval/useApprovalQueue";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ApprovalRule, CostSummary } from "../../editor/api/types";

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// The approval page (ADR-0032, SDD-015): what waits for a person, the rules that approve without
// asking and the cost ceilings. It shows what the service answers and decides nothing; the queue
// updates by itself while the page is open.
export default function Approvals(): ReactNode {
	const t = useT();
	const queue = useApprovalQueue();
	const context = useApprovalContext(queue.requests);
	const [destinations, setDestinations] = useState<Record<string, string>>({});
	const [projects, setProjects] = useState<Record<string, string>>({});
	const [rules, setRules] = useState<ApprovalRule[]>([]);
	const [cost, setCost] = useState<CostSummary | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	const loadRules = useCallback(async (): Promise<void> => {
		try {
			setRules((await api().listApprovalRules()).rules);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, []);

	const loadCost = useCallback(async (): Promise<void> => {
		try {
			setCost(await api().getCostSummary());
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, []);

	useEffect(() => {
		void loadRules();
		void loadCost();
		api()
			.listConnections()
			.then((res) =>
				setDestinations(
					Object.fromEntries(res.connections.map((c) => [c.id, c.label])),
				),
			)
			.catch((error: unknown) => setFailure(failureOf(error)));
		api()
			.listProjects()
			.then((res) =>
				setProjects(
					Object.fromEntries(res.projects.map((p) => [p.id, p.name])),
				),
			)
			.catch((error: unknown) => setFailure(failureOf(error)));
	}, [loadRules, loadCost]);

	return (
		<>
			<Head title={t("ui.approvals.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-6">
				<PageHeader
					title={t("ui.approvals.title")}
					description={t("ui.approvals.intro")}
					actions={
						<Button asChild variant="outline" size="lg">
							<a href="#approvals-rules">
								<SlidersHorizontal aria-hidden="true" />
								{t("ui.approvals.rules_link")}
							</a>
						</Button>
					}
				/>
				<FailureNote failure={failure ?? queue.failure} />
				<div className="grid grid-cols-[minmax(0,1fr)] gap-6 lg:grid-cols-[minmax(0,1fr)_21rem] lg:items-start">
					<ApprovalQueue
						requests={queue.requests ?? []}
						context={context}
						loading={queue.requests === null}
						failed={queue.failure !== null}
						destinations={destinations}
						busyId={queue.busyId}
						live={queue.live}
						onDecide={(id, decision, reason) =>
							void queue.decide(id, decision, reason).then(() => loadCost())
						}
					/>
					<div className="flex flex-col gap-4">
						<div className="rounded-lg border border-border bg-surface p-4">
							<CostPanel
								summary={cost}
								projects={projects}
								onChanged={() => void loadCost()}
							/>
						</div>
						<div className="rounded-lg border border-border bg-surface p-4">
							<RulesPanel
								rules={rules}
								destinations={destinations}
								projects={projects}
								onChanged={() => void loadRules()}
							/>
						</div>
					</div>
				</div>
			</div>
		</>
	);
}
