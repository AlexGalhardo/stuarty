import { Link } from "@inertiajs/react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useLocale, useT } from "@/lib/i18n";
import { formatMoney } from "../dashboard/approval/money";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";
import type {
	Job,
	ModelView,
	ProviderView,
	VoiceView,
} from "../editor/api/types";
import type { ProjectSummary } from "../types";

// The "Generate" dialog of the two sections of the asset library (SDD-016 section 10, epic 10): an image in
// Visual Assets, a narration in Video Assets. The dialog asks for the project, the connection, the model and
// what to make, shows the estimate the service made (a dry run) before anything is submitted, then follows the
// job until it ends and tells the grid to reload. It computes nothing: the estimate, the limits, the waiting
// for a person and the stored files are the service's.

export type Kind = "image" | "voice";

const VOICE_FORMATS = ["mp3", "wav", "flac", "opus", "aac"] as const;
const POLL_MS = 1500;

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

function isFinal(job: Job): boolean {
	return (
		job.state === "succeeded" ||
		job.state === "failed" ||
		job.state === "cancelled"
	);
}

interface Choice {
	connectionId: string;
	label: string;
}

// The connections of the providers that have the capability.
export function choicesOf(providers: ProviderView[], kind: Kind): Choice[] {
	const out: Choice[] = [];
	for (const provider of providers) {
		if (!provider.capabilities.includes(kind)) continue;
		for (const id of provider.connections) {
			out.push({ connectionId: id, label: `${provider.name} (${id})` });
		}
	}
	return out;
}

// The models the picker offers: the ones whose hint is the capability first, then the others, since the hint
// is not a rule and the provider is the judge.
export function orderedModels(models: ModelView[], kind: Kind): ModelView[] {
	const hinted = models.filter((model) => model.capability === kind);
	const rest = models.filter((model) => model.capability !== kind);
	return [...hinted, ...rest];
}

export function GenerateDialog({
	kind,
	open,
	onOpenChange,
	onGenerated,
}: {
	kind: Kind;
	open: boolean;
	onOpenChange(open: boolean): void;
	onGenerated(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
	const [providers, setProviders] = useState<ProviderView[] | null>(null);
	const [project, setProject] = useState("");
	const [connection, setConnection] = useState("");
	const [models, setModels] = useState<ModelView[]>([]);
	const [model, setModel] = useState("");
	const [voices, setVoices] = useState<VoiceView[]>([]);
	const [voice, setVoice] = useState("");
	const [prompt, setPrompt] = useState("");
	const [size, setSize] = useState("1024x1024");
	const [count, setCount] = useState("1");
	const [format, setFormat] = useState<string>("mp3");
	const [language, setLanguage] = useState("");
	const [estimate, setEstimate] = useState<Job | null>(null);
	const [job, setJob] = useState<Job | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const reported = useRef<string | null>(null);

	// What the dialog needs is read when it opens: the projects, and the providers with their connections.
	useEffect(() => {
		if (!open) return;
		let alive = true;
		setFailure(null);
		void (async () => {
			try {
				const [list, provs] = await Promise.all([
					api().listProjects(),
					api().listProviders(),
				]);
				if (!alive) return;
				setProjects(list.projects.filter((p) => p.state === "active"));
				setProviders(provs.providers);
			} catch (error) {
				if (alive) setFailure(failureOf(error));
			}
		})();
		return () => {
			alive = false;
		};
	}, [open]);

	const choices = providers === null ? [] : choicesOf(providers, kind);

	useEffect(() => {
		if (project === "" && projects !== null && projects[0] !== undefined) {
			setProject(projects[0].id);
		}
	}, [project, projects]);

	useEffect(() => {
		if (connection === "" && choices[0] !== undefined) {
			setConnection(choices[0].connectionId);
		}
	}, [connection, choices]);

	// The models, and the voices, of the connection that is chosen.
	useEffect(() => {
		if (!open || connection === "") return;
		let alive = true;
		setEstimate(null);
		void (async () => {
			try {
				const list = await api().listProviderModels(connection);
				if (!alive) return;
				const ordered = orderedModels(list.models, kind);
				setModels(ordered);
				setModel(ordered[0]?.id ?? "");
				if (kind === "voice") {
					const found = await api().listVoices(connection);
					if (!alive) return;
					setVoices(found.voices);
					setVoice(found.voices[0]?.id ?? "");
				}
			} catch (error) {
				if (alive) setFailure(failureOf(error));
			}
		})();
		return () => {
			alive = false;
		};
	}, [open, connection, kind]);

	// The job is followed until it ends; a job that waits for a person is read again, and the grid reloads once.
	const jobId = job?.job_id;
	const done = job !== null && isFinal(job);
	useEffect(() => {
		if (jobId === undefined || done) return;
		const timer = setInterval(() => {
			void (async () => {
				try {
					setJob(await api().getJob(jobId));
				} catch (error) {
					setFailure(failureOf(error));
				}
			})();
		}, POLL_MS);
		return () => clearInterval(timer);
	}, [jobId, done]);

	useEffect(() => {
		if (
			job !== null &&
			job.state === "succeeded" &&
			reported.current !== job.job_id
		) {
			reported.current = job.job_id;
			onGenerated();
		}
	}, [job, onGenerated]);

	const request = useCallback(
		(dryRun: boolean) =>
			kind === "image"
				? api().generateImage({
						project_id: project,
						connection_id: connection,
						model,
						prompt,
						...(size.trim() === "" ? {} : { size: size.trim() }),
						...(count.trim() === "" ? {} : { count: Number(count) }),
						...(dryRun ? { dry_run: true } : {}),
					})
				: api().generateVoice({
						project_id: project,
						connection_id: connection,
						model,
						text: prompt,
						voice,
						format,
						...(language.trim() === "" ? {} : { language: language.trim() }),
						...(dryRun ? { dry_run: true } : {}),
					}),
		[
			kind,
			project,
			connection,
			model,
			prompt,
			size,
			count,
			voice,
			format,
			language,
		],
	);

	async function showEstimate(): Promise<void> {
		setFailure(null);
		setBusy(true);
		try {
			setEstimate(await request(true));
		} catch (error) {
			setEstimate(null);
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setFailure(null);
		setBusy(true);
		try {
			setJob(await request(false));
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	const ready =
		project !== "" &&
		connection !== "" &&
		model !== "" &&
		prompt.trim() !== "" &&
		(kind === "image" || voice !== "");
	const noProvider = providers !== null && choices.length === 0;

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-h-[calc(100vh-48px)] overflow-auto sm:max-w-[560px]">
				<DialogHeader>
					<DialogTitle>{t(`ui.generate.title.${kind}`)}</DialogTitle>
					<DialogDescription>
						{t(`ui.generate.intro.${kind}`)}
					</DialogDescription>
				</DialogHeader>
				{noProvider && (
					<p className="text-sm" data-testid="generate-no-provider">
						{t(`ui.generate.no_provider.${kind}`)}{" "}
						<Link href="/connections" className="underline">
							{t("ui.generate.open_connections")}
						</Link>
					</p>
				)}
				{!noProvider && (
					<form
						onSubmit={(event) => void submit(event)}
						className="flex flex-col gap-3"
						data-generate-kind={kind}
					>
						<div className="flex flex-col gap-1">
							<Label htmlFor={`generate-project-${kind}`}>
								{t("ui.generate.project")}
							</Label>
							<select
								id={`generate-project-${kind}`}
								value={project}
								onChange={(event) => setProject(event.target.value)}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								{(projects ?? []).map((p) => (
									<option key={p.id} value={p.id}>
										{p.name}
									</option>
								))}
							</select>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor={`generate-connection-${kind}`}>
								{t("ui.generate.connection")}
							</Label>
							<select
								id={`generate-connection-${kind}`}
								value={connection}
								onChange={(event) => setConnection(event.target.value)}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								{choices.map((choice) => (
									<option key={choice.connectionId} value={choice.connectionId}>
										{choice.label}
									</option>
								))}
							</select>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor={`generate-model-${kind}`}>
								{t("ui.generate.model")}
							</Label>
							<select
								id={`generate-model-${kind}`}
								value={model}
								onChange={(event) => {
									setModel(event.target.value);
									setEstimate(null);
								}}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								{models.map((m) => (
									<option key={m.id} value={m.id}>
										{m.id}
										{m.price === null ? ` (${t("ui.generate.no_price")})` : ""}
									</option>
								))}
							</select>
						</div>
						{kind === "voice" && (
							<div className="flex flex-col gap-1">
								<Label htmlFor="generate-voice">{t("ui.generate.voice")}</Label>
								<select
									id="generate-voice"
									value={voice}
									onChange={(event) => setVoice(event.target.value)}
									className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
								>
									{voices.map((v) => (
										<option key={v.id} value={v.id}>
											{v.id}
										</option>
									))}
								</select>
							</div>
						)}
						<div className="flex flex-col gap-1">
							<Label htmlFor={`generate-prompt-${kind}`}>
								{t(`ui.generate.prompt.${kind}`)}
							</Label>
							<Textarea
								id={`generate-prompt-${kind}`}
								rows={4}
								value={prompt}
								onChange={(event) => {
									setPrompt(event.target.value);
									setEstimate(null);
								}}
							/>
						</div>
						{kind === "image" ? (
							<div className="flex flex-wrap gap-3">
								<div className="flex flex-col gap-1">
									<Label htmlFor="generate-size">{t("ui.generate.size")}</Label>
									<Input
										id="generate-size"
										className="w-36"
										value={size}
										onChange={(event) => {
											setSize(event.target.value);
											setEstimate(null);
										}}
									/>
								</div>
								<div className="flex flex-col gap-1">
									<Label htmlFor="generate-count">
										{t("ui.generate.count")}
									</Label>
									<Input
										id="generate-count"
										className="w-24"
										inputMode="numeric"
										value={count}
										onChange={(event) => {
											setCount(event.target.value);
											setEstimate(null);
										}}
									/>
								</div>
							</div>
						) : (
							<div className="flex flex-wrap gap-3">
								<div className="flex flex-col gap-1">
									<Label htmlFor="generate-format">
										{t("ui.generate.format")}
									</Label>
									<select
										id="generate-format"
										value={format}
										onChange={(event) => setFormat(event.target.value)}
										className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
									>
										{VOICE_FORMATS.map((f) => (
											<option key={f} value={f}>
												{f}
											</option>
										))}
									</select>
								</div>
								<div className="flex flex-col gap-1">
									<Label htmlFor="generate-language">
										{t("ui.generate.language")}
									</Label>
									<Input
										id="generate-language"
										className="w-28"
										value={language}
										onChange={(event) => setLanguage(event.target.value)}
									/>
								</div>
							</div>
						)}
						{kind === "voice" && (
							<p className="text-xs text-muted-foreground">
								{t("ui.generate.disclosure")}
							</p>
						)}
						<div className="flex flex-wrap items-center gap-3">
							<Button
								type="button"
								variant="outline"
								disabled={!ready || busy || job !== null}
								onClick={() => void showEstimate()}
							>
								{t("ui.generate.estimate")}
							</Button>
							<Button type="submit" disabled={!ready || busy || job !== null}>
								{t("ui.generate.submit")}
							</Button>
						</div>
						{estimate?.estimate !== undefined && job === null && (
							<p className="text-sm" data-testid="generate-estimate">
								{t("ui.generate.estimated", {
									amount: formatMoney(estimate.estimate, locale),
								})}
							</p>
						)}
					</form>
				)}
				<FailureNote failure={failure} />
				{job !== null && (
					<div
						className="flex flex-col gap-1 text-sm"
						aria-live="polite"
						data-testid="generate-job"
						data-job-state={job.state}
					>
						<p>{t(`ui.generate.state.${job.state}`)}</p>
						{job.state === "waiting_approval" && (
							<p>
								<Link href="/approvals" className="underline">
									{t("ui.generate.open_approvals")}
								</Link>
							</p>
						)}
						{job.state === "succeeded" &&
							job.result?.generated !== undefined && (
								<>
									<p>
										{t("ui.generate.done", {
											delivered: String(job.result.generated.delivered),
											requested: String(job.result.generated.requested),
											amount: formatMoney(job.result.generated.cost, locale),
										})}
									</p>
									{(job.result.generated.notes ?? []).map((note) => (
										<p key={note} className="text-muted-foreground">
											<UntrustedText>{note}</UntrustedText>
										</p>
									))}
								</>
							)}
						{job.state === "failed" && job.error !== undefined && (
							<FailureNote
								failure={{
									code: job.error.code,
									message: job.error.message,
									hint: job.error.hint,
								}}
							/>
						)}
					</div>
				)}
			</DialogContent>
		</Dialog>
	);
}
