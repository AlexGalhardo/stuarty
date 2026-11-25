import { Link } from "@inertiajs/react";
import { Ban, Film, Globe, Info, ShieldCheck } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useState } from "react";

import {
	type Failure,
	FailureNote,
	failureIsAbout,
	failureOf,
} from "@/components/Failure";
import { StatusBadge } from "@/components/StatusBadge";
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
import { cn } from "@/lib/cn";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { formatMoney } from "../../dashboard/approval/money";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	Connection,
	Job,
	SocialAccount,
	SocialNetwork,
} from "../../editor/api/types";
import { type TextItem, writingClient } from "../Write/client";
import type { PublishApi } from "./api";
import { useBackgroundAsk } from "./BackgroundAsk";
import { browserZone } from "./calendar";
import type { CalendarApi } from "./calendarApi";
import {
	type ContentKind,
	type Destination,
	destinationsFor,
} from "./destinations";
import { exportLabel } from "./exportLabel";
import { TargetResult } from "./ListView";
import { actionsOf } from "./publicationState";
import { TargetStateBadge } from "./TargetStateBadge";
import type { Publication, Target, TargetInput, TargetState } from "./types";
import { QuietDeclined, useQuietConfirm } from "./useQuietConfirm";

const VISIBILITIES = ["private", "unlisted", "public"] as const;
// A post is a draft (private) or published (public) at its destination.
const POST_VISIBILITIES = ["private", "public"] as const;
const STILLS = ["png", "jpg", "jpeg"];
// The words of what a destination takes that have a translation; any other the service names is drawn as it is.
const TAKES_WORDS = [
	"text",
	"image",
	"video",
	"audio",
	"metadata",
	"document",
	"thumbnail",
];
const SELECT =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2 text-sm";

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

// Where a post planned from an hour or from a slot of the day goes: the account the slot names, else a ready account of
// its network, else the first ready one; the person changes it in the editor.
function accountFor(
	accounts: readonly SocialAccount[],
	initial: EditorInitial,
): SocialAccount | undefined {
	const ready = accounts.filter((a) => a.state === "ready");
	return (
		ready.find((a) => a.connection_id === initial.connection) ??
		ready.find((a) => a.network === initial.network) ??
		ready[0]
	);
}

// What a new publication starts with when it is made from the day (SDD-020 section 30A.6): the date and time of the
// hour that was clicked, and the network or account of the slot.
export interface EditorInitial {
	// YYYY-MM-DDTHH:mm, the form of a datetime-local field
	at: string;
	network?: string;
	connection?: string;
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
	const locale = useLocale();
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
			{network?.price !== undefined && (
				<p className="mt-2 text-xs" data-testid="target-price">
					{t("ui.publish.price", {
						post: formatMoney(
							{
								micros: network.price.post_micros,
								currency: network.price.currency,
							},
							locale,
						),
						link: formatMoney(
							{
								micros: network.price.link_micros,
								currency: network.price.currency,
							},
							locale,
						),
						date: network.price.edited ?? network.price.read,
					})}
				</p>
			)}
			{network?.draft === true && (
				<p className="mt-2 text-xs" data-testid="target-draft-warning">
					{t("ui.publish.draft_warning", { name: network.name })}
				</p>
			)}
		</div>
	);
}

// Visual two-column destination chooser (screen 33f): usable destinations on the left, unavailable ones
// on the right with a plain reason from the service's own capability list, plus a hint for text posts.
export function DestinationChooser({
	contentKind,
	destinations,
	selected,
	disabled,
	reasonOf,
	onSelect,
}: {
	contentKind: ContentKind;
	destinations: readonly Destination[];
	selected: string;
	disabled: boolean;
	reasonOf(d: Destination): string;
	onSelect(id: string): void;
}): ReactNode {
	const t = useT();
	const isPost = contentKind === "text";
	const usable = destinations.filter((d) => d.usable);
	const unavailable = destinations.filter((d) => !d.usable);
	const videoOnly = (d: Destination): boolean =>
		d.takes.includes("video") && !d.takes.includes("text");
	const textOnly = (d: Destination): boolean =>
		d.takes.includes("text") && !d.takes.includes("video");
	const allVideoOnly = unavailable.every(videoOnly);
	const allTextOnly = unavailable.every(textOnly);
	const unavailableHeading = isPost
		? allVideoOnly
			? "ui.publish.editor.require_video"
			: "ui.publish.editor.unavailable"
		: allTextOnly
			? "ui.publish.editor.for_text"
			: "ui.publish.editor.unavailable";
	const showUnavailableSub =
		unavailableHeading !== "ui.publish.editor.unavailable";
	const showAddVideoHint =
		isPost && unavailable.some((d) => d.takes.includes("video"));
	if (destinations.length === 0) return null;
	return (
		<div
			className="grid gap-4 lg:grid-cols-2"
			data-testid="destination-chooser"
		>
			<div className="flex flex-col gap-2" data-testid="destinations-available">
				<div className="flex flex-wrap items-baseline gap-2">
					<h4 className="text-sm font-semibold text-text">
						{t(
							isPost
								? "ui.publish.editor.text_destinations"
								: "ui.publish.editor.video_destinations",
						)}
					</h4>
					<span className="text-xs text-text-muted">
						{t("ui.publish.editor.available_count", { count: usable.length })}
					</span>
				</div>
				{usable.length === 0 ? (
					<p className="rounded-lg border border-dashed p-3 text-xs text-text-muted">
						{t("ui.publish.editor.no_targets")}
					</p>
				) : (
					<ul className="flex flex-col gap-2">
						{usable.map((d) => {
							const chosen = selected === d.id;
							return (
								<li key={d.id}>
									<button
										type="button"
										disabled={disabled}
										aria-pressed={chosen}
										data-testid="destination-card"
										data-destination-id={d.id}
										onClick={() => onSelect(d.id)}
										className={cn(
											"flex w-full items-center justify-between gap-3 rounded-lg border p-3 text-left transition-colors focus-visible:outline-2 focus-visible:outline-ring",
											chosen
												? "border-accent bg-accent-soft"
												: "border-border bg-surface hover:bg-surface-hover",
										)}
									>
										<span className="flex min-w-0 items-center gap-2.5">
											{isPost ? (
												<Globe
													aria-hidden="true"
													className="size-4 shrink-0 text-text-muted"
												/>
											) : (
												<Film
													aria-hidden="true"
													className="size-4 shrink-0 text-text-muted"
												/>
											)}
											<span className="min-w-0">
												<span className="block truncate text-sm font-medium text-text">
													<UntrustedText>{d.label}</UntrustedText>
												</span>
												<span className="block truncate text-xs text-text-muted">
													{d.network}
												</span>
											</span>
										</span>
										<StatusBadge status={chosen ? "info" : "success"}>
											{t(
												chosen
													? "ui.publish.editor.chosen"
													: "ui.publish.editor.available",
											)}
										</StatusBadge>
									</button>
								</li>
							);
						})}
					</ul>
				)}
			</div>
			{unavailable.length > 0 && (
				<div
					className="flex flex-col gap-2"
					data-testid="destinations-unavailable"
				>
					<div className="flex flex-wrap items-baseline gap-2">
						<h4 className="text-sm font-semibold text-text">
							{t(unavailableHeading)}
						</h4>
						{showUnavailableSub && (
							<span className="text-xs text-text-muted">
								{t("ui.publish.editor.unavailable")}
							</span>
						)}
					</div>
					<ul className="flex flex-col gap-2">
						{unavailable.map((d) => (
							<li
								key={d.id}
								data-destination-unavailable={d.id}
								className="flex items-center justify-between gap-3 rounded-lg border border-border bg-surface-sunken/60 p-3 opacity-80"
							>
								<span className="flex min-w-0 items-center gap-2.5">
									<Ban
										aria-hidden="true"
										className="size-4 shrink-0 text-text-subtle"
									/>
									<span className="min-w-0">
										<span className="block truncate text-sm font-medium text-text-muted">
											<UntrustedText>{d.label}</UntrustedText>
										</span>
										<span className="block text-xs text-text-subtle">
											{reasonOf(d)}
										</span>
									</span>
								</span>
								<StatusBadge status="neutral">
									{t(
										isPost && videoOnly(d)
											? "ui.publish.editor.requires_video"
											: "ui.publish.editor.unavailable",
									)}
								</StatusBadge>
							</li>
						))}
					</ul>
					{showAddVideoHint && (
						<p className="flex items-start gap-1.5 text-xs text-text-muted">
							<Info aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
							{t("ui.publish.editor.add_video_unlocks")}
						</p>
					)}
				</div>
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
	blogs = [],
	api,
	calendarApi,
	initial,
	onClose,
	onChanged,
	onDuplicated,
}: {
	// null makes a new publication
	publication: Publication | null;
	// The date and time (and the account) a new publication starts with, when it is made from a day or a slot.
	initial?: EditorInitial;
	calendarApi?: CalendarApi;
	onDuplicated?(copy: Publication): void;
	projects: readonly ProjectSummary[];
	accounts: readonly SocialAccount[];
	networks: readonly SocialNetwork[];
	// The connections of the blog destinations: where a post goes (SDD-020 part 5).
	blogs?: readonly Connection[];
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
	// A publication is a video or a blog post (SDD-020 section 43): a post goes only to the blog destinations.
	const [isPost, setIsPost] = useState((publication?.post ?? "") !== "");
	const [post, setPost] = useState(publication?.post ?? "");
	const [posts, setPosts] = useState<TextItem[]>([]);
	const [drafts, setDrafts] = useState<Draft[]>(() => {
		if (publication !== null) return publication.targets.map(draftOf);
		if (initial === undefined) return [];
		return [
			{
				...blankDraft(accountFor(accounts, initial)?.connection_id ?? "", zone),
				at: initial.at,
			},
		];
	});
	const [saved, setSaved] = useState<Draft[]>(
		publication?.targets.map(draftOf) ?? [],
	);
	const [notes, setNotes] = useState<string[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const backgroundAsk = useBackgroundAsk();
	const quiet = useQuietConfirm();
	const isNew = current === null;

	useEffect(() => {
		if (project === "") return;
		let active = true;
		createClient(new HttpTransport("/api/v1"))
			.listProjectJobs(project)
			.then((list) => {
				if (active) setJobs(list);
			})
			.catch((error: unknown) => {
				if (active) setFailure(failureOf(error));
			});
		return () => {
			active = false;
		};
	}, [project]);

	useEffect(() => {
		if (project === "") return;
		let active = true;
		writingClient(new HttpTransport("/api/v1"))
			.list({ project_id: project, text_kind: "post" })
			.then((res) => {
				if (active) setPosts(res.texts);
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
		setPost(next.post ?? "");
		setIsPost((next.post ?? "") !== "");
		setStill(next.thumbnail ?? "");
		const fresh = next.targets.map(draftOf);
		setDrafts(fresh);
		setSaved(fresh);
		setNotes(next.notes ?? []);
		onChanged();
	}

	// A day marked as quiet is a person's rule (SDD-020 section 30A.1): the service refuses the first call, the person is
	// asked, and only a yes repeats it with the confirmation. `action` receives that flag.
	async function run(
		action: (confirmQuietDay: boolean) => Promise<Publication>,
	): Promise<boolean> {
		setBusy(true);
		setFailure(null);
		try {
			adopt(await quiet.run(action));
			return true;
		} catch (error) {
			if (error instanceof QuietDeclined) return false;
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
		await run((confirm) => {
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
						...(isPost
							? { post }
							: {
									export: video,
									...(still === "" ? {} : { thumbnail: still }),
								}),
						targets,
					})
				: api.update(current.id, {
						title,
						text,
						...(isPost ? { post } : { export: video, thumbnail: still }),
						targets: [...targets, ...removed],
						...(confirm ? { confirm_quiet_day: true } : {}),
					});
		});
	}

	const videos = exportsOf(jobs, null);
	const stills = exportsOf(jobs, STILLS);
	// The core named the kids question as the wrong field: it is marked, and the focus goes to it.
	const kidsMissing = failureIsAbout(failure, "made_for_kids");
	useEffect(() => {
		if (kidsMissing && failure !== null) {
			document
				.querySelector<HTMLInputElement>("[data-kids-missing] input")
				?.focus();
		}
	}, [kidsMissing, failure]);
	const canSave =
		title.trim() !== "" &&
		(isPost ? post !== "" : video !== "") &&
		(current !== null || project !== "");
	// Every destination, the ones that take this content first; the others stay, disabled, with the reason the
	// service's own list of what each takes gives (epic 82).
	const contentKind: ContentKind = isPost ? "text" : "video";
	const destinations = destinationsFor(contentKind, accounts, networks, blogs);
	const firstUsable = destinations.find((d) => d.usable);
	const takesWords = (takes: readonly string[]): string =>
		takes.length === 0
			? t("ui.publish.editor.takes_nothing")
			: takes
					.map((word) =>
						TAKES_WORDS.includes(word)
							? t(`ui.publish.editor.takes_word.${word}`)
							: word,
					)
					.join(", ");
	const reasonOf = (d: Destination): string =>
		d.blocked === "other_family"
			? t(
					contentKind === "text"
						? "ui.publish.editor.post_only_blog"
						: "ui.publish.editor.video_only_network",
				)
			: t("ui.publish.editor.cannot_take", {
					takes: takesWords(d.takes),
					kind: t(`ui.publish.editor.takes_word.${contentKind}`),
				});
	const blocked = destinations.filter((d) => !d.usable);

	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent
				className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"
				data-testid="publication-editor"
			>
				<DialogHeader>
					<DialogTitle>
						{isNew && isPost
							? t("ui.publish.editor.where_post_goes")
							: isNew
								? t("ui.publish.editor.new")
								: t("ui.publish.editor.edit")}
					</DialogTitle>
					<DialogDescription>
						{isPost
							? t("ui.publish.editor.post_intro")
							: t("ui.publish.editor.intro")}
					</DialogDescription>
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
									const next = event.target.value;
									setProject(next);
									const chosenProject = projects.find((p) => p.id === next);
									if (isNew && chosenProject !== undefined) {
										if (chosenProject.kind === "text") setIsPost(true);
										else if (chosenProject.kind === "video") setIsPost(false);
									}
									setVideo("");
									setStill("");
									setPost("");
									setJobs([]);
									setPosts([]);
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
							<Label htmlFor="pub-content">
								{t("ui.publish.editor.content")}
							</Label>
							<select
								id="pub-content"
								className={SELECT}
								disabled={!isNew}
								value={isPost ? "post" : "video"}
								onChange={(event) => setIsPost(event.target.value === "post")}
							>
								<option value="video">
									{t("ui.publish.editor.content_video")}
								</option>
								<option value="post">
									{t("ui.publish.editor.content_post")}
								</option>
							</select>
						</div>
						{isPost && (
							<div className="flex flex-col gap-2">
								<Label htmlFor="pub-post">{t("ui.publish.editor.post")}</Label>
								<select
									id="pub-post"
									className={SELECT}
									value={post}
									onChange={(event) => setPost(event.target.value)}
								>
									<option value="">{t("ui.social.choose")}</option>
									{post !== "" && !posts.some((p) => p.id === post) && (
										<option value={post}>{post}</option>
									)}
									{posts.map((p) => (
										<option key={p.id} value={p.id}>
											{p.title}
										</option>
									))}
								</select>
							</div>
						)}
						{!isPost && (
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
										<option value={video}>
											{t("ui.social.chosen_export")}
										</option>
									)}
									{videos.map((job) => (
										<option key={job.job_id} value={job.job_id}>
											{exportLabel(job, formatDateTime(job.created_at, locale))}
										</option>
									))}
								</select>
							</div>
						)}
						{!isPost && (
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
										<option value={still}>
											{t("ui.social.chosen_thumbnail")}
										</option>
									)}
									{stills.map((job) => (
										<option key={job.job_id} value={job.job_id}>
											{exportLabel(job, formatDateTime(job.created_at, locale))}
										</option>
									))}
								</select>
								<Label htmlFor="pub-still-image">
									{t("ui.social.thumbnail_image")}
								</Label>
								<Input
									id="pub-still-image"
									value={still}
									onChange={(event) => setStill(event.target.value.trim())}
								/>
							</div>
						)}
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
								<DestinationChooser
									contentKind={contentKind}
									destinations={destinations}
									selected={d.connection}
									disabled={!editable}
									reasonOf={reasonOf}
									onSelect={(id) => patch(d.key, { connection: id })}
								/>
								<div className="grid gap-3 sm:grid-cols-2">
									<div className="flex flex-col gap-1">
										<Label htmlFor={`${d.key}-account`}>
											{t("ui.publish.editor.account")}
										</Label>
										<select
											id={`${d.key}-account`}
											className={SELECT}
											value={d.connection}
											aria-describedby={
												blocked.length === 0 ? undefined : `${d.key}-blocked`
											}
											onChange={(event) =>
												patch(d.key, { connection: event.target.value })
											}
										>
											<option value="">{t("ui.social.choose")}</option>
											{destinations.map((a) => (
												<option key={a.id} value={a.id} disabled={!a.usable}>
													{a.label} ({a.network})
													{a.usable ? "" : ` – ${reasonOf(a)}`}
												</option>
											))}
										</select>
										{blocked.length > 0 && (
											<ul
												id={`${d.key}-blocked`}
												className="text-xs text-muted-foreground"
												data-testid="destination-reasons"
											>
												{blocked.map((a) => (
													<li key={a.id} data-destination-id={a.id}>
														<UntrustedText>{a.label}</UntrustedText>
														{": "}
														{reasonOf(a)}
													</li>
												))}
											</ul>
										)}
									</div>
									<div className="flex flex-col gap-1">
										<Label htmlFor={`${d.key}-visibility`}>
											{isPost
												? t("ui.publish.editor.post_status")
												: t("ui.social.visibility")}
										</Label>
										<select
											id={`${d.key}-visibility`}
											className={SELECT}
											value={d.visibility}
											onChange={(event) =>
												patch(d.key, { visibility: event.target.value })
											}
										>
											{isPost
												? POST_VISIBILITIES.map((v) => (
														<option key={v} value={v}>
															{t(`ui.publish.editor.post_visibility_${v}`)}
														</option>
													))
												: VISIBILITIES.map((v) => (
														<option key={v} value={v}>
															{t(`ui.social.visibility_${v}`)}
														</option>
													))}
										</select>
									</div>
								</div>
								<div
									data-testid="target-when"
									className="flex flex-col gap-3 rounded-lg border border-border bg-surface-sunken/40 p-3"
								>
									<h4 className="text-sm font-semibold text-text">
										{t("ui.publish.editor.when")}
									</h4>
									<div className="grid gap-3 sm:grid-cols-2">
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
										{isPost ? (
											<div className="flex flex-col justify-between gap-2 text-xs">
												<div className="flex items-center justify-between border-t border-border pt-2">
													<span className="text-text-muted">
														{t("ui.publish.editor.cost")}
													</span>
													<span className="font-medium text-text">
														{t("ui.publish.editor.free")}
													</span>
												</div>
												<p className="flex items-start gap-1.5 text-text-muted">
													<ShieldCheck
														aria-hidden="true"
														className="mt-0.5 size-3.5 shrink-0"
													/>
													{t("ui.publish.editor.approval_note")}
												</p>
											</div>
										) : (
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
										)}
									</div>
								</div>
								{!isPost && (
									<>
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
											<Label htmlFor={`${d.key}-tags`}>
												{t("ui.social.tags")}
											</Label>
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
											<fieldset
												className={`flex items-center gap-3 ${
													kidsMissing && d.kids === ""
														? "rounded-lg border border-destructive p-2"
														: ""
												}`}
												data-kids-missing={
													kidsMissing && d.kids === "" ? "" : undefined
												}
												aria-describedby={
													kidsMissing && d.kids === ""
														? "publication-failure"
														: undefined
												}
											>
												<legend className="sr-only">
													{t("ui.social.kids")}
												</legend>
												<span aria-hidden="true">{t("ui.social.kids")}</span>
												{(["yes", "no"] as const).map((answer) => (
													<label
														key={answer}
														className="flex items-center gap-1"
													>
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
												{kidsMissing && d.kids === "" && (
													<span className="text-destructive">
														{t("ui.social.kids_required")}
													</span>
												)}
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
									</>
								)}
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
												run((confirm) =>
													api.schedule(current.id, {
														target_id: d.id,
														at: d.at === "" ? undefined : d.at,
														zone: d.zone,
														late_window_minutes: Number(d.late) || 0,
														...(confirm ? { confirm_quiet_day: true } : {}),
													}),
												).then((scheduled) =>
													scheduled ? backgroundAsk.ask() : undefined,
												)
											}
											onSendNow={() =>
												current !== null &&
												d.id !== undefined &&
												run((confirm) =>
													api.sendNow(current.id, d.id ?? "", confirm),
												)
											}
											onRetry={() =>
												current !== null &&
												d.id !== undefined &&
												run((confirm) =>
													api.retry(current.id, d.id ?? "", confirm),
												)
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
									blankDraft(firstUsable?.id ?? "", zone),
								])
							}
						>
							{t("ui.publish.editor.add_target")}
						</Button>
					</div>
					{current !== null && calendarApi !== undefined && (
						<DuplicateBox
							publication={current}
							zone={zone}
							busy={busy}
							calendarApi={calendarApi}
							onDuplicated={(copy) => {
								onChanged();
								onDuplicated?.(copy);
							}}
							onFail={(error) => setFailure(failureOf(error))}
						/>
					)}
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
					<FailureNote failure={failure} id="publication-failure" />
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
				{quiet.dialog}
			</DialogContent>
		</Dialog>
	);
}

// Copy a publication to another day or hour (SDD-020 section 30A.3): the service makes a new publication of drafts at the
// time typed here, with no approval, and leaves the original as it is. The page opens the copy.
function DuplicateBox({
	publication,
	zone,
	busy,
	calendarApi,
	onDuplicated,
	onFail,
}: {
	publication: Publication;
	zone: string;
	busy: boolean;
	calendarApi: CalendarApi;
	onDuplicated(copy: Publication): void;
	onFail(error: unknown): void;
}): ReactNode {
	const t = useT();
	const [at, setAt] = useState("");
	const [working, setWorking] = useState(false);

	async function copy(): Promise<void> {
		setWorking(true);
		try {
			onDuplicated(await calendarApi.duplicate(publication.id, { at, zone }));
		} catch (error) {
			onFail(error);
		} finally {
			setWorking(false);
		}
	}

	return (
		<div
			className="flex flex-wrap items-end gap-2 rounded-lg border p-3"
			data-testid="duplicate-box"
		>
			<div className="flex flex-col gap-1">
				<Label htmlFor="duplicate-at">{t("ui.publish.duplicate.at")}</Label>
				<Input
					id="duplicate-at"
					type="datetime-local"
					value={at}
					data-testid="duplicate-at"
					onChange={(event) => setAt(event.target.value)}
				/>
			</div>
			<Button
				type="button"
				variant="outline"
				disabled={busy || working || at === ""}
				data-testid="duplicate-go"
				onClick={() => void copy()}
			>
				{t("ui.publish.duplicate.go")}
			</Button>
			<p className="basis-full text-xs text-muted-foreground">
				{t("ui.publish.duplicate.help")}
			</p>
		</div>
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
