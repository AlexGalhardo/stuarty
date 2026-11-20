import { Link } from "@inertiajs/react";
import { type ReactNode, useEffect, useMemo, useState } from "react";

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
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { Job, SocialAccount, SocialNetwork } from "../../editor/api/types";
import type { PublishApi } from "./api";
import { useBackgroundAsk } from "./BackgroundAsk";
import { browserZone } from "./calendar";
import { TargetResult } from "./ListView";
import { actionsOf } from "./publicationState";
import { TargetStateBadge } from "./TargetStateBadge";
import type { Publication, Target, TargetInput, TargetState } from "./types";

const VISIBILITIES = ["private", "unlisted", "public"] as const;
const STILLS = ["png", "jpg", "jpeg"];
const SELECT = "h-8 rounded-lg border border-input bg-transparent px-2 text-sm";

// What the person has typed for one target. The text of a time is the one a datetime-local field holds.
interface Draft {
	key: string;
	id?: string;
	state?: TargetState;
	connection: string;
	at: string;
	zone: string;
	late: string;
	title: string;
	description: string;
	tags: string;
	visibility: string;
	kids: "" | "yes" | "no";
	ai: boolean;
}

let counter = 0;

function blankDraft(connection: string, zone: string): Draft {
	counter += 1;
	return {
		key: `new-${counter}`,
		connection,
		at: "",
		zone,
		late: "0",
		title: "",
		description: "",
		tags: "",
		visibility: "private",
		kids: "",
		ai: false,
	};
}

function draftOf(target: Target): Draft {
	return {
		key: target.id,
		id: target.id,
		state: target.state,
		connection: target.connection_id,
		at: (target.local_time ?? "").replace(" ", "T"),
		zone: target.zone,
		late: String(target.late_window_minutes),
		title: target.fields.title ?? "",
		description: target.fields.description ?? "",
		tags: (target.fields.tags ?? []).join(", "),
		visibility: target.fields.visibility ?? "private",
		kids:
			target.fields.made_for_kids === undefined
				? ""
				: target.fields.made_for_kids
					? "yes"
					: "no",
		ai: target.fields.ai_disclosure ?? false,
	};
}

function inputOf(d: Draft, was: Draft | undefined): TargetInput {
	const tags = d.tags
		.split(",")
		.map((tag) => tag.trim())
		.filter((tag) => tag !== "");
	const timeChanged =
		was === undefined || d.at !== was.at || d.zone !== was.zone;
	return {
		...(d.id === undefined ? {} : { id: d.id }),
		connection_id: d.connection,
		fields: {
			...(d.title === "" ? {} : { title: d.title }),
			...(d.description === "" ? {} : { description: d.description }),
			...(tags.length === 0 ? {} : { tags }),
			visibility: d.visibility,
			...(d.kids === "" ? {} : { made_for_kids: d.kids === "yes" }),
			...(d.ai ? { ai_disclosure: true } : {}),
		},
		...(d.at !== "" && timeChanged ? { at: d.at, zone: d.zone } : {}),
		late_window_minutes: Number(d.late) || 0,
	};
}

function exportsOf(jobs: Job[], formats: string[] | null): Job[] {
	return jobs.filter((job) => {
		if (job.kind !== "export" || job.state !== "succeeded") return false;
		const format = job.result?.export?.format ?? "mp4";
		return formats === null ? format === "mp4" : formats.includes(format);
	});
}

// The preview of a post on one network: the title and the text as the network will show them, with what the
// network allows. The numbers are read from the network's description; the service validates at scheduling.
function Preview({
	network,
	title,
	description,
	tags,
	visibility,
}: {
	network: SocialNetwork | undefined;
	title: string;
	description: string;
	tags: string;
	visibility: string;
}): ReactNode {
	const t = useT();
	const limits = network?.limits;
	return (
		<div
			className="rounded-lg border bg-muted/40 p-3 text-sm"
			data-testid="target-preview"
		>
			<p className="text-xs text-muted-foreground">
				{t("ui.publish.preview")} · {network?.name ?? ""} ·{" "}
				{t(`ui.social.visibility_${visibility}`)}
			</p>
			<p className="mt-1 break-words font-medium">
				<UntrustedText>{title}</UntrustedText>
			</p>
			{limits !== undefined && (
				<p className="text-xs text-muted-foreground">
					{t("ui.publish.title_count", {
						count: [...title].length,
						max: limits.title_max,
					})}
				</p>
			)}
			<p className="mt-1 whitespace-pre-wrap break-words">
				<UntrustedText>{description}</UntrustedText>
			</p>
			{limits !== undefined && (
				<p className="text-xs text-muted-foreground">
					{t("ui.publish.description_count", {
						count: new TextEncoder().encode(description).length,
						max: limits.description_max_bytes,
					})}
				</p>
			)}
			{tags.trim() !== "" && (
				<p className="mt-1 text-xs">
					<UntrustedText>{tags}</UntrustedText>
				</p>
			)}
		</div>
	);
}

// The editor of a publication (SDD-020 section 30): what it publishes, and one panel for each account with its
// time, its fields, a preview, its state and what it came to. It sends what the person typed and shows what the
// service answers; every rule of a time, a post and an approval is the service's.
export function PublicationEditor({
	publication,
	projects,
	accounts,
	networks,
	api,
	onClose,
	onChanged,
}: {
	// null makes a new publication
	publication: Publication | null;
	projects: readonly ProjectSummary[];
	accounts: readonly SocialAccount[];
	networks: readonly SocialNetwork[];
	api: PublishApi;
	onClose(): void;
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const zone = useMemo(() => browserZone(), []);
	const zones = useMemo(() => {
		const list: string[] =
			typeof Intl.supportedValuesOf === "function"
				? Intl.supportedValuesOf("timeZone")
				: [];
		return list.includes(zone) ? list : [zone, ...list];
	}, [zone]);
	const [current, setCurrent] = useState<Publication | null>(publication);
	const [project, setProject] = useState(publication?.project_id ?? "");
	const [jobs, setJobs] = useState<Job[]>([]);
	const [title, setTitle] = useState(publication?.title ?? "");
	const [text, setText] = useState(publication?.text ?? "");
	const [video, setVideo] = useState(publication?.export ?? "");
	const [still, setStill] = useState(publication?.thumbnail ?? "");
	const [drafts, setDrafts] = useState<Draft[]>(
		publication?.targets.map(draftOf) ?? [],
	);
	const [saved, setSaved] = useState<Draft[]>(
		publication?.targets.map(draftOf) ?? [],
	);
	const [notes, setNotes] = useState<string[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const backgroundAsk = useBackgroundAsk();
	const ready = accounts.filter((a) => a.state === "ready");
	const isNew = current === null;

	useEffect(() => {
		if (project === "") return;
		let active = true;
		createClient(new HttpTransport("/api/v1"))
			.listJobs(project)
			.then((res) => {
				if (active) setJobs(res.jobs);
			})
			.catch((error: unknown) => {
				if (active) setFailure(failureOf(error));
			});
		return () => {
			active = false;
		};
	}, [project]);

	function adopt(next: Publication): void {
		setCurrent(next);
		setTitle(next.title);
		setText(next.text ?? "");
		setVideo(next.export);
		setStill(next.thumbnail ?? "");
		const fresh = next.targets.map(draftOf);
		setDrafts(fresh);
		setSaved(fresh);
		setNotes(next.notes ?? []);
		onChanged();
	}

	async function run(action: () => Promise<Publication>): Promise<boolean> {
		setBusy(true);
		setFailure(null);
		try {
			adopt(await action());
			return true;
		} catch (error) {
			setFailure(failureOf(error));
			return false;
		} finally {
			setBusy(false);
		}
	}

	function patch(key: string, change: Partial<Draft>): void {
		setDrafts((was) =>
			was.map((d) => (d.key === key ? { ...d, ...change } : d)),
		);
	}

	async function save(): Promise<void> {
		await run(() => {
			const targets = drafts.map((d) =>
				inputOf(
					d,
					saved.find((s) => s.key === d.key),
				),
			);
			const removed = saved
				.filter(
					(s) => s.id !== undefined && !drafts.some((d) => d.key === s.key),
				)
				.map((s): TargetInput => ({ id: s.id, remove: true }));
			return current === null
				? api.create({
						project_id: project,
						title,
						...(text === "" ? {} : { text }),
						export: video,
						...(still === "" ? {} : { thumbnail: still }),
						targets,
					})
				: api.update(current.id, {
						title,
						text,
						export: video,
						thumbnail: still,
						targets: [...targets, ...removed],
					});
		});
	}

	const videos = exportsOf(jobs, null);
	const stills = exportsOf(jobs, STILLS);
	const canSave =
		title.trim() !== "" && video !== "" && (current !== null || project !== "");

	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent
				className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"
				data-testid="publication-editor"
			>
				<DialogHeader>
					<DialogTitle>
						{isNew ? t("ui.publish.editor.new") : t("ui.publish.editor.edit")}
					</DialogTitle>
					<DialogDescription>{t("ui.publish.editor.intro")}</DialogDescription>
				</DialogHeader>
				<form
					className="flex flex-col gap-4"
					onSubmit={(event) => {
						event.preventDefault();
						void save();
					}}
				>
					<div className="grid gap-4 sm:grid-cols-2">
						<div className="flex flex-col gap-2">
							<Label htmlFor="pub-project">{t("ui.social.project")}</Label>
							<select
								id="pub-project"
								className={SELECT}
								disabled={!isNew}
								value={project}
								onChange={(event) => {
									setProject(event.target.value);
									setVideo("");
									setStill("");
									setJobs([]);
								}}
							>
								<option value="">{t("ui.social.choose")}</option>
								{projects.map((p) => (
									<option key={p.id} value={p.id}>
										{p.name}
									</option>
								))}
							</select>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="pub-title">{t("ui.social.post_title")}</Label>
							<Input
								id="pub-title"
								required
								autoComplete="off"
								value={title}
								onChange={(event) => setTitle(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="pub-video">{t("ui.social.video")}</Label>
							<select
								id="pub-video"
								className={SELECT}
								value={video}
								onChange={(event) => setVideo(event.target.value)}
							>
								<option value="">{t("ui.social.choose")}</option>
								{video !== "" && !videos.some((j) => j.job_id === video) && (
									<option value={video}>{video}</option>
								)}
								{videos.map((job) => (
									<option key={job.job_id} value={job.job_id}>
										{job.job_id} · {formatDateTime(job.created_at, locale)}
									</option>
								))}
							</select>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="pub-still">{t("ui.social.thumbnail")}</Label>
							<select
								id="pub-still"
								className={SELECT}
								value={still}
								onChange={(event) => setStill(event.target.value)}
							>
								<option value="">{t("ui.social.no_thumbnail")}</option>
								{still !== "" && !stills.some((j) => j.job_id === still) && (
									<option value={still}>{still}</option>
								)}
								{stills.map((job) => (
									<option key={job.job_id} value={job.job_id}>
										{job.job_id} · {formatDateTime(job.created_at, locale)}
									</option>
								))}
							</select>
						</div>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="pub-text">{t("ui.publish.editor.caption")}</Label>
						<Textarea
							id="pub-text"
							value={text}
							onChange={(event) => setText(event.target.value)}
						/>
					</div>

					<h3 className="text-sm font-semibold">
						{t("ui.publish.editor.targets")}
					</h3>
					{drafts.length === 0 && (
						<p className="text-sm text-muted-foreground">
							{t("ui.publish.editor.no_targets")}
						</p>
					)}
					{drafts.map((d, index) => {
						const target = current?.targets.find((x) => x.id === d.id);
						const actions = actionsOf(d.state ?? "draft");
						const network = networks.find(
							(n) =>
								n.network ===
								accounts.find((a) => a.connection_id === d.connection)?.network,
						);
						const editable = d.id === undefined || actions.edit;
						return (
							<fieldset
								key={d.key}
								className="flex flex-col gap-3 rounded-lg border p-3"
								disabled={!editable}
								data-testid="target-panel"
								data-target-id={d.id}
							>
								<legend className="flex items-center gap-2 px-1 text-sm font-medium">
									{t("ui.publish.editor.target", { number: index + 1 })}
									{d.state !== undefined && (
										<TargetStateBadge state={d.state} />
									)}
								</legend>
								<div className="grid gap-3 sm:grid-cols-2">
									<div className="flex flex-col gap-1">
										<Label htmlFor={`${d.key}-account`}>
											{t("ui.publish.editor.account")}
										</Label>
										<select
											id={`${d.key}-account`}
											className={SELECT}
											value={d.connection}
											onChange={(event) =>
												patch(d.key, { connection: event.target.value })
											}
										>
											<option value="">{t("ui.social.choose")}</option>
											{ready.map((a) => (
												<option key={a.connection_id} value={a.connection_id}>
													{a.label} ({a.network})
												</option>
											))}
										</select>
									</div>
									<div className="flex flex-col gap-1">
										<Label htmlFor={`${d.key}-visibility`}>
											{t("ui.social.visibility")}
										</Label>
										<select
											id={`${d.key}-visibility`}
											className={SELECT}
											value={d.visibility}
											onChange={(event) =>
												patch(d.key, { visibility: event.target.value })
											}
										>
											{VISIBILITIES.map((v) => (
												<option key={v} value={v}>
													{t(`ui.social.visibility_${v}`)}
												</option>
											))}
										</select>
									</div>
									<div className="flex flex-col gap-1">
										<Label htmlFor={`${d.key}-at`}>
											{t("ui.publish.editor.at")}
										</Label>
										<Input
											id={`${d.key}-at`}
											type="datetime-local"
											value={d.at}
											onChange={(event) =>
												patch(d.key, { at: event.target.value })
											}
										/>
									</div>
									<div className="flex flex-col gap-1">
										<Label htmlFor={`${d.key}-zone`}>
											{t("ui.publish.editor.zone")}
										</Label>
										<Input
											id={`${d.key}-zone`}
											list="publish-zones"
											autoComplete="off"
											value={d.zone}
											onChange={(event) =>
												patch(d.key, { zone: event.target.value })
											}
										/>
									</div>
									<div className="flex flex-col gap-1">
										<Label htmlFor={`${d.key}-late`}>
											{t("ui.publish.editor.late")}
										</Label>
										<Input
											id={`${d.key}-late`}
											type="number"
											min={0}
											max={1440}
											value={d.late}
											onChange={(event) =>
												patch(d.key, { late: event.target.value })
											}
										/>
										<p className="text-xs text-muted-foreground">
											{t("ui.publish.editor.late_help")}
										</p>
									</div>
									<div className="flex flex-col gap-1">
										<Label htmlFor={`${d.key}-title`}>
											{t("ui.publish.editor.target_title")}
										</Label>
										<Input
											id={`${d.key}-title`}
											autoComplete="off"
											value={d.title}
											onChange={(event) =>
												patch(d.key, { title: event.target.value })
											}
										/>
									</div>
								</div>
								<div className="flex flex-col gap-1">
									<Label htmlFor={`${d.key}-description`}>
										{t("ui.publish.editor.target_description")}
									</Label>
									<Textarea
										id={`${d.key}-description`}
										value={d.description}
										onChange={(event) =>
											patch(d.key, { description: event.target.value })
										}
									/>
								</div>
								<div className="flex flex-col gap-1">
									<Label htmlFor={`${d.key}-tags`}>{t("ui.social.tags")}</Label>
									<Input
										id={`${d.key}-tags`}
										autoComplete="off"
										value={d.tags}
										onChange={(event) =>
											patch(d.key, { tags: event.target.value })
										}
									/>
								</div>
								<div className="flex flex-wrap items-center gap-4 text-sm">
									<fieldset className="flex items-center gap-3">
										<legend className="sr-only">{t("ui.social.kids")}</legend>
										<span aria-hidden="true">{t("ui.social.kids")}</span>
										{(["yes", "no"] as const).map((answer) => (
											<label key={answer} className="flex items-center gap-1">
												<input
													type="radio"
													name={`${d.key}-kids`}
													className="accent-primary"
													checked={d.kids === answer}
													onChange={() => patch(d.key, { kids: answer })}
												/>
												{t(`ui.social.kids_${answer}`)}
											</label>
										))}
									</fieldset>
									<label className="flex items-center gap-2">
										<input
											type="checkbox"
											className="accent-primary"
											checked={d.ai}
											onChange={(event) =>
												patch(d.key, { ai: event.target.checked })
											}
										/>
										{t("ui.social.synthetic")}
									</label>
								</div>
								<Preview
									network={network}
									title={d.title || title}
									description={d.description || text}
									tags={d.tags}
									visibility={d.visibility}
								/>
								{target !== undefined && (
									<div className="flex flex-col gap-2 text-sm">
										<TargetResult target={target} />
										{actions.approve && (
											<p data-testid="target-waits">
												{t("ui.publish.editor.waits")}{" "}
												<Link
													href="/approvals"
													className="underline underline-offset-4"
												>
													{t("ui.social.open_approvals")}
												</Link>
											</p>
										)}
									</div>
								)}
								<div className="flex flex-wrap gap-2">
									{d.id === undefined ? (
										<Button
											type="button"
											variant="outline"
											onClick={() =>
												setDrafts((was) => was.filter((x) => x.key !== d.key))
											}
										>
											{t("ui.publish.editor.remove")}
										</Button>
									) : (
										<TargetButtons
											target={target}
											busy={busy}
											dirty={
												JSON.stringify(d) !==
												JSON.stringify(saved.find((s) => s.key === d.key))
											}
											onSchedule={() =>
												current !== null &&
												run(() =>
													api.schedule(current.id, {
														target_id: d.id,
														at: d.at === "" ? undefined : d.at,
														zone: d.zone,
														late_window_minutes: Number(d.late) || 0,
													}),
												).then((scheduled) =>
													scheduled ? backgroundAsk.ask() : undefined,
												)
											}
											onSendNow={() =>
												current !== null &&
												d.id !== undefined &&
												run(() => api.sendNow(current.id, d.id ?? ""))
											}
											onRetry={() =>
												current !== null &&
												d.id !== undefined &&
												run(() => api.retry(current.id, d.id ?? ""))
											}
											onCancel={() =>
												current !== null &&
												d.id !== undefined &&
												run(() => api.cancel(current.id, d.id ?? ""))
											}
										/>
									)}
								</div>
							</fieldset>
						);
					})}
					<datalist id="publish-zones">
						{zones.map((z) => (
							<option key={z} value={z} />
						))}
					</datalist>
					<div>
						<Button
							type="button"
							variant="outline"
							onClick={() =>
								setDrafts((was) => [
									...was,
									blankDraft(ready[0]?.connection_id ?? "", zone),
								])
							}
						>
							{t("ui.publish.editor.add_target")}
						</Button>
					</div>
					{notes.map((note) => (
						<p
							key={note}
							role="status"
							className="rounded-lg border bg-muted p-3 text-sm"
							data-testid="publication-note"
						>
							<UntrustedText>{note}</UntrustedText>
						</p>
					))}
					<FailureNote failure={failure} />
					<div className="flex flex-wrap gap-2">
						<Button type="submit" disabled={busy || !canSave}>
							{t("ui.publish.editor.save")}
						</Button>
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.social.close")}
						</Button>
					</div>
				</form>
				{backgroundAsk.dialog}
			</DialogContent>
		</Dialog>
	);
}

// The buttons a target offers in its state. The service refuses what its state does not allow; the page only does
// not offer it.
function TargetButtons({
	target,
	busy,
	dirty,
	onSchedule,
	onSendNow,
	onRetry,
	onCancel,
}: {
	target: Target | undefined;
	busy: boolean;
	dirty: boolean;
	onSchedule(): void;
	onSendNow(): void;
	onRetry(): void;
	onCancel(): void;
}): ReactNode {
	const t = useT();
	if (target === undefined) return null;
	const actions = actionsOf(target.state);
	return (
		<>
			{actions.schedule && (
				<Button
					type="button"
					disabled={busy || dirty}
					title={dirty ? t("ui.publish.editor.save_first") : undefined}
					onClick={onSchedule}
					data-testid="target-schedule"
				>
					{t("ui.publish.editor.schedule")}
				</Button>
			)}
			{actions.sendNow && (
				<Button
					type="button"
					variant="outline"
					disabled={busy || dirty}
					onClick={onSendNow}
					data-testid="target-send-now"
				>
					{t("ui.publish.editor.send_now")}
				</Button>
			)}
			{actions.retry && (
				<Button
					type="button"
					variant="outline"
					disabled={busy}
					onClick={onRetry}
					data-testid="target-retry"
				>
					{t("ui.publish.editor.retry")}
				</Button>
			)}
			{actions.cancel && (
				<Button
					type="button"
					variant="outline"
					disabled={busy}
					onClick={onCancel}
					data-testid="target-cancel"
				>
					{t("ui.publish.editor.cancel")}
				</Button>
			)}
		</>
	);
}
