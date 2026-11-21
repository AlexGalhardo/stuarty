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
import { type ApiClient, createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";
import { scopedToTimeline, type Transport } from "../editor/api/transport";
import type {
	Job,
	ModelView,
	ProviderView,
	VoiceView,
} from "../editor/api/types";
import type { ProjectSummary } from "../types";

// The "Generate" dialog of the sections of the asset library (SDD-016 section 10, epics 10, 11 and 58): an image
// in Visual Assets, a video clip in Video Assets, a narration in Sound Assets. The dialog asks for the project, the connection, the model and
// what to make, shows the estimate the service made (a dry run) before anything is submitted, then follows the
// job until it ends and tells the grid to reload. It computes nothing: the estimate, the limits, the waiting
// for a person and the stored files are the service's.

export type Kind = "image" | "voice" | "video";

const VOICE_FORMATS = ["mp3", "wav", "flac", "opus", "aac"] as const;
// The aspect ratios the service accepts for a clip (SDD-016 section 5.4); the model may make only some, and the
// service says which when it refuses one.
const VIDEO_ASPECTS = ["16:9", "9:16", "1:1", "4:3", "3:4"] as const;
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

// The connections of the providers that have the capability, named as the person named them: the provider, and
// the name of the connection when it says more; two that read the same are told apart by their order, never by
// an id (VV-325).
export function choicesOf(
	providers: ProviderView[],
	kind: Kind,
	names: Readonly<Record<string, string>> = {},
): Choice[] {
	const out: Choice[] = [];
	for (const provider of providers) {
		if (!provider.capabilities.includes(kind)) continue;
		for (const id of provider.connections) {
			const given = names[id];
			const label =
				given === undefined || given === "" || given === provider.name
					? provider.name
					: `${provider.name} (${given})`;
			out.push({ connectionId: id, label });
		}
	}
	const seen = new Map<string, number>();
	return out.map((choice) => {
		const same = out.filter((other) => other.label === choice.label).length;
		if (same === 1) return choice;
		const n = (seen.get(choice.label) ?? 0) + 1;
		seen.set(choice.label, n);
		return { ...choice, label: `${choice.label} ${n}` };
	});
}

// The timeline whose files a clip of this production may start from: the service keeps a generation under the
// first active timeline, by position (SDD-003 section 1.7 rule 6), and the items come in that order. A production
// with no timeline has none (VV-334).
export function libraryOf(
	items: { id: string; kind: string; state: string; timeline_id?: string }[],
): string | null {
	const first = items.find(
		(item) => item.kind === "timeline" && item.state === "active",
	);
	return first === undefined ? null : (first.timeline_id ?? first.id);
}

// The images a generation left in a production with no timeline, which wait in it for the first video (SDD-005
// section 5.12.3): only the library lists them. The identifier of a file is the one it has in this production, and
// the poster of an entry may be another production's, so that one is read from the uses of the file.
async function imagesWaitingIn(
	client: ApiClient,
	production: string,
): Promise<string[]> {
	const found = await client.queryAssets({
		kind: "image",
		project: production,
	});
	const ids = await Promise.all(
		found.assets.map(async (entry) => {
			if (entry.poster?.timeline_id === production) {
				return entry.poster.asset_id;
			}
			const full = await client.getAsset(entry.hash);
			return full.uses?.find((use) => use.project_id === production)?.asset_id;
		}),
	);
	return ids.filter((id) => id !== undefined);
}

// The images a clip of this production may start from, as the service will look for them. The first timeline has
// the id of its project, so with two of them only `timeline_id` says which one is listed (VV-334).
export async function firstFrames(
	transport: Transport,
	production: string,
): Promise<string[]> {
	const client = createClient(transport);
	const timeline = libraryOf((await client.describeProject(production)).items);
	if (timeline === null) return imagesWaitingIn(client, production);
	const { assets } = await createClient(
		scopedToTimeline(transport, timeline),
	).listMedia(timeline);
	return assets
		.filter((a) => a.kind === "image" && a.state === "ready" && !a.trashed)
		.map((a) => a.asset_id);
}

// Whether a provider of the product has the capability at all, connected or not: with none (no provider makes
// video yet, SDD-016 section 2.3) there is nothing to connect, so the dialog does not send the person to look.
export function anyMaker(providers: ProviderView[], kind: Kind): boolean {
	return providers.some((provider) => provider.capabilities.includes(kind));
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
	const [names, setNames] = useState<Record<string, string>>({});
	const [models, setModels] = useState<ModelView[]>([]);
	const [model, setModel] = useState("");
	const [voices, setVoices] = useState<VoiceView[]>([]);
	const [voice, setVoice] = useState("");
	const [prompt, setPrompt] = useState("");
	const [size, setSize] = useState("1024x1024");
	const [count, setCount] = useState("1");
	const [format, setFormat] = useState<string>("mp3");
	const [language, setLanguage] = useState("");
	const [seconds, setSeconds] = useState("5");
	const [aspect, setAspect] = useState<string>("16:9");
	const [firstFrame, setFirstFrame] = useState("");
	const [frames, setFrames] = useState<string[]>([]);
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
				const [list, provs, conns] = await Promise.all([
					api().listProjects(),
					api().listProviders(),
					api().listConnections(),
				]);
				if (!alive) return;
				setNames(
					Object.fromEntries(conns.connections.map((c) => [c.id, c.label])),
				);
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

	const choices = providers === null ? [] : choicesOf(providers, kind, names);

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

	// The images of the project a clip may start from: the ones the library has ready and not in the trash.
	useEffect(() => {
		if (!open || kind !== "video" || project === "") return;
		let alive = true;
		setFirstFrame("");
		setFrames([]);
		void (async () => {
			try {
				const ids = await firstFrames(new HttpTransport("/api/v1"), project);
				if (!alive) return;
				setFrames(ids);
			} catch {
				// The first frame is optional: a list that cannot be read leaves the clip without one.
				if (alive) setFrames([]);
			}
		})();
		return () => {
			alive = false;
		};
	}, [open, kind, project]);

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
		(dryRun: boolean) => {
			const base = {
				project_id: project,
				connection_id: connection,
				model,
				...(dryRun ? { dry_run: true } : {}),
			};
			if (kind === "image") {
				return api().generateImage({
					...base,
					prompt,
					...(size.trim() === "" ? {} : { size: size.trim() }),
					...(count.trim() === "" ? {} : { count: Number(count) }),
				});
			}
			if (kind === "video") {
				return api().generateVideo({
					...base,
					prompt,
					...(seconds.trim() === "" ? {} : { seconds: Number(seconds) }),
					aspect,
					...(firstFrame === "" ? {} : { first_frame: firstFrame }),
				});
			}
			return api().generateVoice({
				...base,
				text: prompt,
				voice,
				format,
				...(language.trim() === "" ? {} : { language: language.trim() }),
			});
		},
		[
			kind,
			project,
			connection,
			model,
			prompt,
			size,
			count,
			seconds,
			aspect,
			firstFrame,
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
		(kind !== "voice" || voice !== "");
	const noProvider = providers !== null && choices.length === 0;
	const noMaker = providers !== null && !anyMaker(providers, kind);

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
						{noMaker ? (
							t(`ui.generate.no_maker.${kind}`)
						) : (
							<>
								{t(`ui.generate.no_provider.${kind}`)}{" "}
								<Link href="/connections" className="underline">
									{t("ui.generate.open_connections")}
								</Link>
							</>
						)}
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
						) : kind === "video" ? (
							<div className="flex flex-wrap gap-3">
								<div className="flex flex-col gap-1">
									<Label htmlFor="generate-seconds">
										{t("ui.generate.seconds")}
									</Label>
									<Input
										id="generate-seconds"
										className="w-24"
										inputMode="numeric"
										value={seconds}
										onChange={(event) => {
											setSeconds(event.target.value);
											setEstimate(null);
										}}
									/>
								</div>
								<div className="flex flex-col gap-1">
									<Label htmlFor="generate-aspect">
										{t("ui.generate.aspect")}
									</Label>
									<select
										id="generate-aspect"
										value={aspect}
										onChange={(event) => {
											setAspect(event.target.value);
											setEstimate(null);
										}}
										className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
									>
										{VIDEO_ASPECTS.map((a) => (
											<option key={a} value={a}>
												{a}
											</option>
										))}
									</select>
								</div>
								<div className="flex flex-col gap-1">
									<Label htmlFor="generate-first-frame">
										{t("ui.generate.first_frame")}
									</Label>
									<select
										id="generate-first-frame"
										value={firstFrame}
										onChange={(event) => {
											setFirstFrame(event.target.value);
											setEstimate(null);
										}}
										className="h-8 max-w-56 rounded-lg border border-input bg-background px-2 text-sm"
									>
										<option value="">{t("ui.generate.no_first_frame")}</option>
										{frames.map((id) => (
											<option key={id} value={id}>
												{id}
											</option>
										))}
									</select>
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
						{kind === "video" && (
							<p
								className="text-xs text-muted-foreground"
								data-testid="generate-always-asks"
							>
								{t("ui.generate.always_asks")}
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
