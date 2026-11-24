import { Link } from "@inertiajs/react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import RenameAssetDialog from "@/components/RenameAssetDialog";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote } from "@/lib/session";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";
import type {
	AssetQuery,
	AssetResult,
	LibraryAsset,
} from "../editor/api/types";
import { AssetSize } from "./AssetSize";
import { OriginBadge } from "./badges";
import { GenerateDialog, type Kind as GenerateKind } from "./GenerateDialog";
import { LibraryFilters } from "./LibraryFilters";
import { LibraryFrame } from "./LibraryFrame";
import {
	LibraryEmpty,
	LibraryLoading,
	LibraryReadFailed,
} from "./LibraryStates";
import { MediaCard } from "./MediaCard";
import { type RevealNotice, RevealNoticeText, useReveal } from "./reveal";
import { SoundImport } from "./SoundImport";
import { SoundRow } from "./SoundRow";

// The three sections of the asset library (SDD-011 section 14.6, RF-320 to RF-323, D-121): Visual Assets, Video
// Assets and Sound Assets are one library seen through a filter on the kind. The page draws what the service answers and sends
// what the person does; it decides nothing: the filters, the search, the uses and the rules of an origin or a
// tag are the service's.

export type Section = "visual" | "video" | "audio";

const NAV_KEY: Record<Section, string> = {
	visual: "visual-assets",
	video: "video-assets",
	audio: "sound-assets",
};

// What the button of each section generates: pictures, a clip, a voice-over (the voices land in Sound Assets).
const GENERATES: Record<Section, GenerateKind> = {
	visual: "image",
	video: "video",
	audio: "voice",
};
const PAGE = 48;

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

function Field({
	label,
	children,
}: {
	label: string;
	children: ReactNode;
}): ReactNode {
	return (
		<div className="flex flex-col gap-1">
			<h3 className="text-sm font-semibold text-text-muted">{label}</h3>
			<div className="text-sm break-words whitespace-pre-wrap">{children}</div>
		</div>
	);
}

// The detail of one file: where it came from, the user's own notes, the projects that use it, and the few
// things a person can change. Every change is a call; the panel shows what the call answered.
export function AssetDetail({
	hash,
	renames = 0,
	onChanged,
	onClose,
}: {
	hash: string;
	// Counts the renames made from the list while this file is open: the title reads the name again.
	renames?: number;
	onChanged(): void;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const id = useId();
	const [asset, setAsset] = useState<AssetResult | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [description, setDescription] = useState("");
	const [rights, setRights] = useState("");
	const [tag, setTag] = useState("");
	const [projects, setProjects] = useState<{ id: string; name: string }[]>([]);
	const [target, setTarget] = useState("");
	// The timelines of the chosen project, when it holds more than one: the service says which one is missing,
	// the person picks it here.
	const [timelines, setTimelines] = useState<{ id: string; title: string }[]>(
		[],
	);
	const [timeline, setTimeline] = useState("");
	// Use waits while the timelines of the chosen project are being read, and an answer about a project the
	// person has already left is dropped: either would send the file with no timeline, or to the wrong one.
	const [readingTimelines, setReadingTimelines] = useState(false);
	const chosenProject = useRef("");
	const [said, setSaid] = useState<string | null>(null);
	const reveal = useReveal();
	const [renaming, setRenaming] = useState(false);
	const [notice, setNotice] = useState<RevealNotice | null>(null);
	const renameButton = useRef<HTMLButtonElement>(null);

	// The text boxes take the service's text when the file is opened and after Save; a tag or the reuse mark
	// that changes while a person is typing must not take what they typed.
	const show = useCallback((result: AssetResult, resetText = true): void => {
		setAsset(result);
		if (resetText) {
			setDescription(result.description ?? "");
			setRights(result.origin?.rights_note ?? "");
		}
	}, []);

	useEffect(() => {
		let live = true;
		setAsset(null);
		setSaid(null);
		setNotice(null);
		setFailure(null);
		void (async () => {
			try {
				const [result, list] = await Promise.all([
					api().getAsset(hash),
					api().listProjects(),
				]);
				if (!live) return;
				show(result);
				setProjects(list.projects.map((p) => ({ id: p.id, name: p.name })));
			} catch (error) {
				if (live) setFailure(failureOf(error));
			}
		})();
		return () => {
			live = false;
		};
	}, [hash, show]);

	// Keeps what the person is typing in the description and the rights note.
	useEffect(() => {
		if (renames === 0) return;
		let live = true;
		void api()
			.getAsset(hash)
			.then((result) => {
				if (live) show(result, false);
			})
			.catch((error: unknown) => {
				if (live) setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, [renames, hash, show]);

	async function change(
		call: () => Promise<AssetResult>,
		message: string,
		resetText = false,
	): Promise<void> {
		setFailure(null);
		setSaid(null);
		try {
			show(await call(), resetText);
			setSaid(message);
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	// Opens the folder (desktop) or copies the path (browser). The page sends the hash; the service checks the
	// session, the file and the path, and a refusal is shown as it came.
	async function showFolder(): Promise<void> {
		setFailure(null);
		setSaid(null);
		setNotice(null);
		const outcome = await reveal.run(hash);
		if ("failure" in outcome) setFailure(outcome.failure);
		else setNotice(outcome.notice);
	}

	async function chooseProject(projectId: string): Promise<void> {
		chosenProject.current = projectId;
		setTarget(projectId);
		setTimeline("");
		setTimelines([]);
		setFailure(null);
		setReadingTimelines(true);
		try {
			const described = await api().describeProject(projectId);
			if (chosenProject.current !== projectId) return;
			const found = described.items
				.filter((item) => item.kind === "timeline" && item.state === "active")
				.map((item) => ({
					id: item.timeline_id ?? item.id,
					title: item.title,
				}));
			setTimelines(found.length > 1 ? found : []);
		} catch (error) {
			if (chosenProject.current === projectId) setFailure(failureOf(error));
		} finally {
			if (chosenProject.current === projectId) setReadingTimelines(false);
		}
	}

	const cannotUse =
		target === "" ||
		readingTimelines ||
		(timelines.length > 0 && timeline === "");

	async function addToProject(
		event: FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault();
		if (cannotUse) return;
		setFailure(null);
		setSaid(null);
		try {
			const result = await api().useAsset(
				target,
				hash,
				timelines.length > 0 ? timeline : undefined,
			);
			const entry = result.assets[0];
			if (entry?.error !== undefined) {
				setFailure({
					code: entry.error.code,
					message: entry.error.message,
					hint: entry.error.hint,
				});
				return;
			}
			setSaid(t("ui.assets.detail.used"));
			show(await api().getAsset(hash));
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	// Moves the file to the trash of one project that holds it (MediaTrash); the library lists what a project
	// still holds, so a file that no project holds any more leaves the list and the panel closes.
	async function trash(projectId: string, assetId: string): Promise<void> {
		setFailure(null);
		setSaid(null);
		try {
			await api().trashMedia(projectId, assetId);
			onChanged();
			try {
				show(await api().getAsset(hash));
				setSaid(t("ui.assets.detail.trashed"));
			} catch {
				onClose();
			}
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	return (
		<aside
			aria-labelledby={`${id}-title`}
			className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-4 text-text"
			data-testid="asset-detail"
		>
			<div className="flex items-start justify-between gap-2">
				<h2
					id={`${id}-title`}
					className="min-w-0 truncate text-lg font-semibold"
				>
					{asset === null ? (
						t("ui.assets.detail.title")
					) : (
						<UntrustedText>{asset.name}</UntrustedText>
					)}
				</h2>
				<Button type="button" variant="outline" onClick={onClose}>
					{t("ui.assets.detail.close")}
				</Button>
			</div>
			<FailureNote failure={failure} />
			{said !== null && (
				<p role="status" className="text-sm text-text-muted">
					{said}
				</p>
			)}
			<RevealNoticeText notice={notice} />
			{asset !== null && (
				<>
					<Field label={t("ui.assets.detail.origin")}>
						<p data-testid="asset-origin">
							<OriginBadge kind={asset.origin?.kind} />
						</p>
						{asset.origin?.provider !== undefined && (
							<p>
								{t("ui.assets.detail.provider", {
									provider: asset.origin.provider,
									model: asset.origin.model ?? "",
								})}
							</p>
						)}
						{asset.origin?.cost_micros !== undefined && (
							<p>
								{t("ui.assets.detail.cost", {
									amount: new Intl.NumberFormat(locale, {
										maximumFractionDigits: 4,
									}).format(asset.origin.cost_micros / 1_000_000),
								})}
							</p>
						)}
						{asset.origin?.source !== undefined && (
							<p>
								<UntrustedText>{asset.origin.source}</UntrustedText>
							</p>
						)}
						{asset.origin?.prompt !== undefined && (
							<p>
								<UntrustedText>{asset.origin.prompt}</UntrustedText>
							</p>
						)}
					</Field>
					<Field label={t("ui.assets.detail.facts")}>
						<p data-testid="asset-detail-size">
							<AssetSize asset={asset} />
						</p>
						<p>
							{[
								t(`ui.assets.kind.${asset.kind}`),
								asset.width !== undefined && asset.height !== undefined
									? `${asset.width}x${asset.height}`
									: null,
								asset.created_at !== undefined
									? t("ui.assets.detail.created", {
											date: formatDateTime(asset.created_at, locale),
										})
									: null,
							]
								.filter((part) => part !== null)
								.join(" · ")}
						</p>
						<p data-testid="asset-shared">
							{t("ui.assets.detail.used_by", { count: asset.projects })}
							{asset.projects > 1 && ` ${t("ui.assets.detail.counted_once")}`}
						</p>
						<p className="text-text-muted">
							{t("ui.assets.detail.file_on_disk")}
						</p>
						<div className="mt-1 flex flex-wrap items-center gap-2">
							<Button
								ref={renameButton}
								type="button"
								variant="outline"
								onClick={() => setRenaming(true)}
							>
								{t("ui.editor.library.rename.action")}
							</Button>
							{reveal.kind !== null && (
								<Button
									type="button"
									variant="outline"
									disabled={reveal.disabled}
									aria-describedby={
										reveal.disabled ? `${id}-person-only` : undefined
									}
									onClick={() => void showFolder()}
								>
									{t(
										reveal.kind === "show"
											? "ui.assets.reveal.show"
											: "ui.assets.reveal.copy",
									)}
								</Button>
							)}
						</div>
						{reveal.disabled && <PersonOnlyNote id={`${id}-person-only`} />}
						<RenameAssetDialog
							open={renaming}
							onOpenChange={setRenaming}
							name={asset.name}
							rename={async (name) => {
								show(await api().renameAsset(hash, name), false);
								onChanged();
							}}
							returnFocus={() => renameButton.current}
						/>
					</Field>
					<Field label={t("ui.assets.detail.rights_summary")}>
						<p data-testid="asset-rights">
							{asset.origin?.rights_note !== undefined &&
							asset.origin.rights_note !== "" ? (
								<UntrustedText>{asset.origin.rights_note}</UntrustedText>
							) : (
								t("ui.assets.detail.no_rights")
							)}
						</p>
						<p data-testid="asset-reuse">
							{asset.reusable === true
								? t("ui.assets.detail.reuse_yes")
								: t("ui.assets.detail.reuse_no")}
						</p>
					</Field>
					<div className="flex flex-col gap-2">
						<Label htmlFor={`${id}-description`}>
							{t("ui.assets.detail.description")}
						</Label>
						<Textarea
							id={`${id}-description`}
							maxLength={2000}
							value={description}
							onChange={(event) => setDescription(event.target.value)}
						/>
						<Label htmlFor={`${id}-rights`}>
							{t("ui.assets.detail.rights")}
						</Label>
						<Textarea
							id={`${id}-rights`}
							maxLength={2000}
							value={rights}
							onChange={(event) => setRights(event.target.value)}
						/>
						<p className="text-sm text-text-muted">
							{t("ui.assets.detail.rights_notice")}
						</p>
						<label className="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								className="accent-primary"
								checked={asset.reusable === true}
								onChange={(event) =>
									void change(
										() =>
											api().setAssetInfo(hash, {
												reusable: event.target.checked,
											}),
										t("ui.assets.detail.saved"),
									)
								}
							/>
							{t("ui.assets.detail.reusable")}
						</label>
						<div>
							<Button
								type="button"
								onClick={() =>
									void change(
										() =>
											api().setAssetInfo(hash, {
												description,
												rights_note: rights,
											}),
										t("ui.assets.detail.saved"),
										true,
									)
								}
							>
								{t("ui.assets.detail.save")}
							</Button>
						</div>
					</div>
					<div className="flex flex-col gap-2">
						<h3 className="text-sm font-semibold text-text-muted">
							{t("ui.assets.detail.tags")}
						</h3>
						<ul className="flex flex-wrap gap-2">
							{(asset.tags ?? []).map((name) => (
								<li
									key={name}
									className="flex items-center gap-1 rounded-full bg-surface-hover px-2 py-1 text-sm text-text"
								>
									<UntrustedText>{name}</UntrustedText>
									<button
										type="button"
										className="px-1"
										onClick={() =>
											void change(
												() => api().tagAsset(hash, [], [name]),
												t("ui.assets.detail.saved"),
											)
										}
									>
										<span aria-hidden="true">×</span>
										<span className="sr-only">
											{t("ui.assets.detail.tag_remove", { tag: name })}
										</span>
									</button>
								</li>
							))}
						</ul>
						<form
							className="flex gap-2"
							onSubmit={(event) => {
								event.preventDefault();
								const added = tag;
								setTag("");
								void change(
									() => api().tagAsset(hash, [added], []),
									t("ui.assets.detail.saved"),
								);
							}}
						>
							<Label htmlFor={`${id}-tag`} className="sr-only">
								{t("ui.assets.detail.tag_add_label")}
							</Label>
							<Input
								id={`${id}-tag`}
								autoComplete="off"
								maxLength={40}
								value={tag}
								onChange={(event) => setTag(event.target.value)}
							/>
							<Button
								type="submit"
								variant="outline"
								disabled={tag.trim() === ""}
							>
								{t("ui.assets.detail.tag_add")}
							</Button>
						</form>
					</div>
					<div className="flex flex-col gap-2">
						<h3 className="text-sm font-semibold text-text-muted">
							{t("ui.assets.detail.uses")}
						</h3>
						{(asset.uses ?? []).length === 0 ? (
							<p className="text-sm text-text-muted">
								{t("ui.assets.detail.no_uses")}
							</p>
						) : (
							<ul
								className="flex flex-col gap-1 text-sm"
								data-testid="asset-uses"
							>
								{(asset.uses ?? []).map((use) => (
									<li
										key={use.project_id}
										className="flex flex-wrap items-center gap-2"
									>
										<Link
											href={`/projects/${use.project_id}`}
											className="underline"
										>
											<UntrustedText>{use.name}</UntrustedText>
										</Link>
										{asset.kind === "audio" && (
											<Button
												type="button"
												variant="outline"
												className="h-auto max-w-full whitespace-normal py-1 text-left"
												onClick={() => void trash(use.project_id, use.asset_id)}
											>
												{t("ui.assets.detail.trash", { project: use.name })}
											</Button>
										)}
									</li>
								))}
							</ul>
						)}
						<form
							className="flex flex-col gap-2"
							onSubmit={(event) => void addToProject(event)}
						>
							<fieldset className="flex max-h-40 flex-col gap-1 overflow-auto">
								<legend className="mb-1 text-sm font-medium">
									{t("ui.assets.detail.use_in")}
								</legend>
								{projects.map((project) => (
									<label
										key={project.id}
										className="flex items-center gap-2 text-sm"
									>
										<input
											type="radio"
											name={`${id}-project`}
											className="accent-primary"
											checked={target === project.id}
											onChange={() => void chooseProject(project.id)}
										/>
										<UntrustedText>{project.name}</UntrustedText>
									</label>
								))}
							</fieldset>
							{timelines.length > 0 && (
								<fieldset
									className="flex max-h-40 flex-col gap-1 overflow-auto"
									data-testid="asset-use-timelines"
								>
									<legend className="mb-1 text-sm font-medium">
										{t("ui.assets.detail.use_timeline")}
									</legend>
									{timelines.map((tl) => (
										<label
											key={tl.id}
											className="flex items-center gap-2 text-sm"
										>
											<input
												type="radio"
												name={`${id}-timeline`}
												className="accent-primary"
												checked={timeline === tl.id}
												onChange={() => setTimeline(tl.id)}
											/>
											<UntrustedText>{tl.title}</UntrustedText>
										</label>
									))}
								</fieldset>
							)}
							<div>
								<Button type="submit" variant="outline" disabled={cannotUse}>
									{t("ui.assets.detail.use")}
								</Button>
							</div>
						</form>
					</div>
				</>
			)}
		</aside>
	);
}

// One section of the library: the filters, the files and the detail of the file that is open.
export function AssetLibrary({ section }: { section: Section }): ReactNode {
	const t = useT();
	const [items, setItems] = useState<LibraryAsset[] | null>(null);
	const [total, setTotal] = useState(0);
	const [next, setNext] = useState<string | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [applied, setApplied] = useState<AssetQuery>({});
	const [open, setOpen] = useState<string | null>(null);
	const [renames, setRenames] = useState(0);
	// What the whole library of this section weighs, from the service: the page adds nothing up.
	const [summary, setSummary] = useState<{
		files: number;
		size: string;
		unsized: number;
	}>({ files: 0, size: "", unsized: 0 });
	const reveal = useReveal();
	// The refusal or the confirmation of a menu action on a card or a row.
	const [actionFailure, setActionFailure] = useState<Failure | null>(null);
	const [notice, setNotice] = useState<RevealNotice | null>(null);
	// The dialog stays mounted so that it closes the way a dialog does; each opening is a new one (a new key),
	// so a second generation starts from an empty form.
	const [generating, setGenerating] = useState<{
		open: boolean;
		count: number;
		kind: GenerateKind;
	}>({ open: false, count: 0, kind: "image" });

	const load = useCallback(
		async (query: AssetQuery, cursor?: string): Promise<void> => {
			try {
				const result = await api().queryAssets({
					...query,
					section,
					limit: PAGE,
					cursor,
				});
				setItems((was) =>
					cursor === undefined
						? result.assets
						: [...(was ?? []), ...result.assets],
				);
				setTotal(result.total);
				setSummary({
					files: result.file_count,
					size: result.total_size_text,
					unsized: result.unsized,
				});
				setNext(result.next_cursor);
			} catch (error) {
				setFailure(failureOf(error));
			}
		},
		[section],
	);

	useEffect(() => {
		setFailure(null);
		void load(applied);
	}, [applied, load]);

	const filtered = Object.keys(applied).length > 0;
	const renamed = (): void => {
		setRenames((n) => n + 1);
		void load(applied);
	};
	// "Show in folder" or "Copy path" from a card or a row: the hash goes to the service, the answer is a notice or
	// the refusal as it came.
	const showFolder = async (hash: string): Promise<void> => {
		setActionFailure(null);
		setNotice(null);
		const outcome = await reveal.run(hash);
		if ("failure" in outcome) setActionFailure(outcome.failure);
		else setNotice(outcome.notice);
	};

	return (
		<LibraryFrame
			navKey={NAV_KEY[section]}
			description={t(`ui.assets.intro.${section}`)}
			actions={
				<Button
					type="button"
					onClick={() =>
						setGenerating((was) => ({
							open: true,
							count: was.count + 1,
							kind: GENERATES[section],
						}))
					}
				>
					{t(`ui.generate.open.${section}`)}
				</Button>
			}
		>
			{generating.count > 0 && (
				<GenerateDialog
					key={generating.count}
					kind={generating.kind}
					open={generating.open}
					onOpenChange={(next) =>
						setGenerating((was) => ({ ...was, open: next }))
					}
					onGenerated={() => void load(applied)}
				/>
			)}
			{section === "audio" && (
				<SoundImport onImported={() => void load(applied)} />
			)}
			<LibraryFilters section={section} onApply={setApplied} />
			<FailureNote failure={failure} />
			<div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
				<section aria-labelledby={`files-${section}`}>
					<h2
						id={`files-${section}`}
						className="text-sm text-text-muted"
						hidden={items === null}
					>
						{t("ui.assets.count", { count: String(total) })}
					</h2>
					{items !== null && summary.files > 0 && (
						<p className="text-sm text-text-muted" data-testid="library-total">
							{summary.size === ""
								? t("ui.assets.size.unknown")
								: t("ui.assets.total", { size: summary.size })}
							{summary.unsized > 0 &&
								summary.size !== "" &&
								` ${t("ui.assets.unsized", { count: summary.unsized })}`}
						</p>
					)}
					<FailureNote failure={actionFailure} />
					<RevealNoticeText notice={notice} />
					{items === null && failure === null && (
						<LibraryLoading section={section} />
					)}
					{items === null && failure !== null && (
						<LibraryReadFailed
							section={section}
							onRetry={() => {
								setFailure(null);
								void load(applied);
							}}
						/>
					)}
					{items !== null && items.length === 0 && filtered && (
						<p
							className="mt-4 text-sm text-text-muted"
							data-testid="asset-empty"
						>
							{t("ui.assets.empty_filtered")}
						</p>
					)}
					{items !== null && items.length === 0 && !filtered && (
						<div data-testid="asset-empty">
							<LibraryEmpty section={section} />
						</div>
					)}
					{items !== null && items.length > 0 && section === "audio" && (
						<ul className="mt-3 flex flex-col gap-3" data-testid="asset-grid">
							{items.map((asset) => (
								<SoundRow
									key={asset.hash}
									asset={asset}
									selected={open === asset.hash}
									onOpen={() => setOpen(asset.hash)}
									rename={async (name) => {
										await api().renameAsset(asset.hash, name);
										renamed();
									}}
									reveal={reveal}
									onReveal={(hash) => void showFolder(hash)}
								/>
							))}
						</ul>
					)}
					{items !== null && items.length > 0 && section !== "audio" && (
						<ul
							className="mt-3 grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-3"
							data-testid="asset-grid"
						>
							{items.map((asset) => (
								<MediaCard
									key={asset.hash}
									asset={asset}
									selected={open === asset.hash}
									onOpen={() => setOpen(asset.hash)}
									onRenamed={renamed}
									reveal={reveal}
									onReveal={(hash) => void showFolder(hash)}
								/>
							))}
						</ul>
					)}
					{next !== null && (
						<div className="mt-4">
							<Button
								type="button"
								variant="outline"
								onClick={() => void load(applied, next)}
							>
								{t("ui.assets.more")}
							</Button>
						</div>
					)}
				</section>
				{open !== null && (
					<AssetDetail
						hash={open}
						renames={renames}
						onChanged={() => void load(applied)}
						onClose={() => setOpen(null)}
					/>
				)}
			</div>
		</LibraryFrame>
	);
}
