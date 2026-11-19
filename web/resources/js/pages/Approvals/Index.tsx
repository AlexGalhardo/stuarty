import { Head } from "@inertiajs/react";
import { type ReactNode, useCallback, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { useT } from "@/lib/i18n";
import ApprovalQueue from "../../dashboard/approval/ApprovalQueue";
import CostPanel from "../../dashboard/approval/CostPanel";
import RulesPanel from "../../dashboard/approval/RulesPanel";
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
	const [destinations, setDestinations] = useState<Record<string, string>>({});
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
	}, [loadRules, loadCost]);

	return (
		<>
			<Head title={t("ui.approvals.title")} />
			<div className="mx-auto flex max-w-3xl flex-col gap-8">
				<div>
					<h1 className="text-2xl font-semibold">{t("ui.approvals.title")}</h1>
					<p className="mt-1 text-sm text-muted-foreground">
						{t("ui.approvals.intro")}
					</p>
				</div>
				<FailureNote failure={failure ?? queue.failure} />
				<ApprovalQueue
					requests={queue.requests ?? []}
					destinations={destinations}
					busyId={queue.busyId}
					live={queue.live}
					onDecide={(id, decision, reason) =>
						void queue.decide(id, decision, reason).then(() => loadCost())
					}
				/>
				<RulesPanel
					rules={rules}
					destinations={destinations}
					onChanged={() => void loadRules()}
				/>
				<CostPanel summary={cost} onChanged={() => void loadCost()} />
			</div>
		</>
	);
}
