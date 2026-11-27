import {
	type FormEvent,
	type ReactNode,
	useEffect,
	useMemo,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { FieldLabel } from "@/components/FieldHelp";
import { Notice } from "@/components/Notice";
import { OptionSelect } from "@/components/OptionSelect";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useLocale, useT } from "@/lib/i18n";
import { formatMoney } from "../../dashboard/approval/money";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { Connection, ModelView } from "../../editor/api/types";
import type { DraftJob, TextItem, WritingClient } from "./client";
import { aiTextConnections } from "./draftConnections";

// How often a job that waits or runs is read again.
export const POLL_MS = 1500;
const FINAL = new Set(["succeeded", "failed", "cancelled"]);
const NETWORKS = ["youtube", "instagram", "facebook", "x", "tiktok"];
// The text actions the panel starts from: they fill the instruction, and the person can change it before asking.
export const PRESETS = ["draft", "rewrite", "shorten"] as const;

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
		Promise.all([api.listProviders(), api.listConnections()])
			.then(([providers, result]) => {
				if (!live) return;
				const text = aiTextConnections(providers.providers, result.connections);
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

	return (
		<section
			aria-labelledby="draft-heading"
			className="flex flex-col gap-3 border-t border-border pt-4"
			data-testid="draft-panel"
		>
			<h3 id="draft-heading" className="text-base font-semibold text-text">
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
						<div className="flex min-w-0 max-w-full flex-col gap-1">
							<span className="text-sm font-medium text-text">
								{t("ui.write.draft.connection")}
							</span>
							<OptionSelect
								label={t("ui.write.draft.connection")}
								value={connection}
								onChange={setConnection}
								options={connections.map((c) => ({
									value: c.id,
									label: c.label,
								}))}
							/>
						</div>
						<div className="flex min-w-0 max-w-full flex-col gap-1">
							<span className="text-sm font-medium text-text">
								{t("ui.write.draft.model")}
							</span>
							<OptionSelect
								label={t("ui.write.draft.model")}
								value={model}
								disabled={models === null}
								onChange={setModel}
								options={(models ?? []).map((m) => ({
									value: m.id,
									label: m.id,
								}))}
							/>
						</div>
					</div>
					{item.text_kind === "caption" && (
						<div className="flex min-w-0 max-w-full flex-col gap-1">
							<span className="text-sm font-medium text-text">
								{t("ui.write.draft.network")}
							</span>
							<OptionSelect
								label={t("ui.write.draft.network")}
								value={network === "" ? "whole" : network}
								onChange={(next) => setNetwork(next === "whole" ? "" : next)}
								options={[
									{ value: "whole", label: t("ui.write.draft.whole") },
									...NETWORKS.map((id) => ({ value: id, label: id })),
								]}
							/>
						</div>
					)}
					<fieldset className="flex min-w-0 flex-wrap items-center gap-2">
						<legend className="sr-only">{t("ui.write.draft.presets")}</legend>
						{PRESETS.map((id) => (
							<Button
								key={id}
								type="button"
								variant="outline"
								size="sm"
								onClick={() =>
									setInstruction(t(`ui.write.draft.preset_text.${id}`))
								}
							>
								{t(`ui.write.draft.preset.${id}`)}
							</Button>
						))}
					</fieldset>
					<div className="flex min-w-0 max-w-full flex-col gap-1">
						<FieldLabel
							htmlFor="draft-instruction"
							label={t("ui.write.draft.instruction")}
							help={t("ui.help.g2.write_draft_help")}
						/>
						<Textarea
							id="draft-instruction"
							placeholder={t("ui.help.g2.write_draft_ph")}
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
			<DraftStatus job={job} estimate={estimate} />
		</section>
	);
}

// What the draft is doing, as the service reported the job: the estimate that comes first, the wait for approval,
// the run, the result with its cost, or the failure. The words of an error are text from the provider.
export function DraftStatus({
	job,
	estimate,
}: {
	job: DraftJob | null;
	estimate: DraftJob | null;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const waiting = job?.state === "waiting_approval";
	const running = job !== null && !FINAL.has(job.state) && !waiting;
	const text = job?.result?.text;
	const cost = estimate?.estimate ?? job?.estimate;
	return (
		<div
			role="status"
			aria-live="polite"
			className="flex flex-col gap-1 text-sm"
		>
			{cost !== undefined && (
				<p data-testid="draft-estimate">
					{t("ui.write.draft.estimated", { cost: formatMoney(cost, locale) })}
				</p>
			)}
			{waiting && (
				<Notice
					tone="warning"
					title={t("ui.write.draft.waiting")}
					data-testid="draft-waiting"
					actions={
						<a className="underline" href="/approvals">
							{t("ui.write.draft.open_approvals")}
						</a>
					}
				>
					{t("ui.write.draft.waiting_body")}
				</Notice>
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
				<p role="alert" className="text-danger-text">
					{t("ui.write.draft.failed")}{" "}
					{job.error !== undefined && (
						<UntrustedText>{job.error.message}</UntrustedText>
					)}
				</p>
			)}
		</div>
	);
}
