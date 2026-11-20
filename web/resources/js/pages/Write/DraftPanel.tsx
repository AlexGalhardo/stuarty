import {
	type FormEvent,
	type ReactNode,
	useEffect,
	useMemo,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useLocale, useT } from "@/lib/i18n";
import { formatMoney } from "../../dashboard/approval/money";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { Connection, ModelView } from "../../editor/api/types";
import type { DraftJob, TextItem, WritingClient } from "./client";

// How often a job that waits or runs is read again.
export const POLL_MS = 1500;
const FINAL = new Set(["succeeded", "failed", "cancelled"]);
const NETWORKS = ["youtube", "instagram", "facebook", "x", "tiktok"];

// The draft with a model (SDD-019 sections 6 and 11): the person picks a connection and a model, says what to write
// and sees the estimate before anything is asked. The job waits in the approval queue like any costly generation;
// when it succeeds the draft is a new revision of the text and the panel says which, and what it cost.
export default function DraftPanel({
	client,
	item,
	onDone,
}: {
	client: WritingClient;
	item: TextItem;
	onDone: () => void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const api = useMemo(() => createClient(new HttpTransport("/api/v1")), []);
	const [connections, setConnections] = useState<Connection[] | null>(null);
	const [connection, setConnection] = useState("");
	const [models, setModels] = useState<ModelView[] | null>(null);
	const [model, setModel] = useState("");
	const [instruction, setInstruction] = useState("");
	const [network, setNetwork] = useState("");
	const [job, setJob] = useState<DraftJob | null>(null);
	const [estimate, setEstimate] = useState<DraftJob | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		let live = true;
		api
			.listConnections()
			.then((result) => {
				if (!live) return;
				const text = result.connections.filter((c) =>
					c.accepts.includes("text"),
				);
				setConnections(text);
				setConnection((current) => current || (text[0]?.id ?? ""));
			})
			.catch((error: unknown) => {
				if (live) setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, [api]);

	useEffect(() => {
		if (connection === "") {
			setModels(null);
			return;
		}
		let live = true;
		setModels(null);
		api
			.listProviderModels(connection, false)
			.then((result) => {
				if (!live) return;
				setModels(result.models);
				setModel((current) =>
					result.models.some((m) => m.id === current)
						? current
						: (result.models[0]?.id ?? ""),
				);
			})
			.catch((error: unknown) => {
				if (live) setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, [api, connection]);

	// A job that waits for approval or runs is read until it ends; the timer dies with the panel.
	const jobId = job?.job_id;
	const jobState = job?.state;
	useEffect(() => {
		if (jobId === undefined || jobState === undefined || FINAL.has(jobState))
			return;
		let live = true;
		const timer = window.setTimeout(() => {
			client
				.job(jobId)
				.then((next) => {
					if (!live) return;
					setJob(next);
					if (next.state === "succeeded") onDone();
				})
				.catch((error: unknown) => {
					if (live) setFailure(failureOf(error));
				});
		}, POLL_MS);
		return () => {
			live = false;
			window.clearTimeout(timer);
		};
	}, [client, jobId, jobState, onDone]);

	async function ask(dryRun: boolean): Promise<void> {
		setBusy(true);
		setFailure(null);
		try {
			const result = await client.draft(item.project_id, item.id, {
				connection_id: connection,
				model,
				instruction,
				network: network === "" ? undefined : network,
				dry_run: dryRun || undefined,
			});
			if (dryRun) {
				setEstimate(result);
			} else {
				setJob(result);
				setEstimate(null);
				if (result.state === "succeeded") onDone();
			}
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	function submit(event: FormEvent<HTMLFormElement>): void {
		event.preventDefault();
		void ask(false);
	}

	const waiting = job?.state === "waiting_approval";
	const running = job !== null && !FINAL.has(job.state) && !waiting;
	const text = job?.result?.text;

	return (
		<section
			aria-labelledby="draft-heading"
			className="flex flex-col gap-2"
			data-testid="draft-panel"
		>
			<h3 id="draft-heading" className="font-medium">
				{t("ui.write.draft.heading")}
			</h3>
			<p className="text-sm text-muted-foreground">
				{t("ui.write.draft.intro")}
			</p>
			{connections !== null && connections.length === 0 && (
				<p className="text-sm">
					{t("ui.write.draft.no_connection")}{" "}
					<a className="underline" href="/connections">
						{t("ui.write.draft.open_connections")}
					</a>
				</p>
			)}
			{connections !== null && connections.length > 0 && (
				<form className="flex flex-col gap-3" onSubmit={submit}>
					<div className="grid gap-3 sm:grid-cols-2">
						<div className="flex flex-col gap-1">
							<Label htmlFor="draft-connection">
								{t("ui.write.draft.connection")}
							</Label>
							<select
								id="draft-connection"
								value={connection}
								onChange={(event) => setConnection(event.target.value)}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								{connections.map((c) => (
									<option key={c.id} value={c.id}>
										{c.label}
									</option>
								))}
							</select>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="draft-model">{t("ui.write.draft.model")}</Label>
							<select
								id="draft-model"
								value={model}
								disabled={models === null}
								onChange={(event) => setModel(event.target.value)}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								{(models ?? []).map((m) => (
									<option key={m.id} value={m.id}>
										{m.id}
									</option>
								))}
							</select>
						</div>
					</div>
					{item.text_kind === "caption" && (
						<div className="flex flex-col gap-1">
							<Label htmlFor="draft-network">
								{t("ui.write.draft.network")}
							</Label>
							<select
								id="draft-network"
								value={network}
								onChange={(event) => setNetwork(event.target.value)}
								className="h-8 w-fit rounded-lg border border-input bg-background px-2 text-sm"
							>
								<option value="">{t("ui.write.draft.whole")}</option>
								{NETWORKS.map((id) => (
									<option key={id} value={id}>
										{id}
									</option>
								))}
							</select>
						</div>
					)}
					<div className="flex flex-col gap-1">
						<Label htmlFor="draft-instruction">
							{t("ui.write.draft.instruction")}
						</Label>
						<Textarea
							id="draft-instruction"
							rows={3}
							value={instruction}
							onChange={(event) => setInstruction(event.target.value)}
						/>
					</div>
					<div className="flex flex-wrap gap-2">
						<Button
							type="button"
							variant="outline"
							disabled={busy || model === "" || instruction.trim() === ""}
							onClick={() => void ask(true)}
						>
							{t("ui.write.draft.estimate")}
						</Button>
						<Button
							type="submit"
							disabled={busy || model === "" || instruction.trim() === ""}
						>
							{t("ui.write.draft.start")}
						</Button>
					</div>
				</form>
			)}
			<FailureNote failure={failure} />
			<div
				role="status"
				aria-live="polite"
				className="flex flex-col gap-1 text-sm"
			>
				{(estimate?.estimate ?? job?.estimate) !== undefined && (
					<p data-testid="draft-estimate">
						{t("ui.write.draft.estimated", {
							cost: formatMoney(
								(estimate?.estimate ?? job?.estimate) as {
									micros: number;
									currency: string;
								},
								locale,
							),
						})}
					</p>
				)}
				{waiting && (
					<p data-testid="draft-waiting">
						{t("ui.write.draft.waiting")}{" "}
						<a className="underline" href="/approvals">
							{t("ui.write.draft.open_approvals")}
						</a>
					</p>
				)}
				{running && <p>{t("ui.write.draft.running")}</p>}
				{job?.state === "succeeded" && text !== undefined && (
					<p data-testid="draft-done">
						{t("ui.write.draft.done", {
							revision: String(text.revision ?? ""),
							cost: formatMoney(text.cost, locale),
						})}
					</p>
				)}
				{(job?.state === "failed" || job?.state === "cancelled") && (
					<p role="alert" className="text-destructive">
						{t("ui.write.draft.failed")}{" "}
						{job.error !== undefined && (
							<UntrustedText>{job.error.message}</UntrustedText>
						)}
					</p>
				)}
			</div>
		</section>
	);
}
