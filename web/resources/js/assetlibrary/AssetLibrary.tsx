import { Head, Link } from "@inertiajs/react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useId,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";
import type {
	AssetQuery,
	AssetResult,
	LibraryAsset,
	OriginKind,
} from "../editor/api/types";
import { thumbnailUrl } from "../editor/api/urls";
import { GenerateDialog, type Kind as GenerateKind } from "./GenerateDialog";

// The two sections of the asset library (SDD-011 section 14.6, RF-320 to RF-323): Visual Assets and Video
// Assets are one library seen through a filter on the kind. The page draws what the service answers and sends
// what the person does; it decides nothing: the filters, the search, the uses and the rules of an origin or a
// tag are the service's.

export type Section = "visual" | "video";

const ORIGINS: readonly OriginKind[] = [
	"imported",
	"generated",
	"fetched_by_agent",
	"rendered_externally",
];
const PAGE = 48;
const POSTER_WIDTH = 320;

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

function sizeText(bytes: number, locale: string): string {
	const units = ["B", "KB", "MB", "GB"];
	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < units.length - 1) {
		value /= 1024;
		unit += 1;
	}
	return `${new Intl.NumberFormat(locale, { maximumFractionDigits: unit === 0 ? 0 : 1 }).format(value)} ${units[unit]}`;
}

function hasPicture(asset: LibraryAsset): boolean {
	return (
		asset.poster !== undefined &&
		(asset.kind === "image" || asset.kind === "video")
	);
}

export function AssetCard({
	asset,
	selected,
	onOpen,
}: {
	asset: LibraryAsset;
	selected: boolean;
	onOpen(): void;
}): ReactNode {
	const t = useT();
	return (
		<li>
			<button
				type="button"
				onClick={onOpen}
				aria-pressed={selected}
				data-asset-hash={asset.hash}
				className="flex w-full flex-col gap-2 rounded-xl border bg-card p-2 text-left text-card-foreground hover:bg-muted focus-visible:outline-2 aria-pressed:border-primary"
			>
				<span className="flex aspect-video items-center justify-center overflow-hidden rounded-lg bg-muted text-xs text-muted-foreground">
					{hasPicture(asset) && asset.poster !== undefined ? (
						<img
							src={thumbnailUrl(
								asset.poster.timeline_id,
								asset.poster.asset_id,
								POSTER_WIDTH,
							)}
							alt=""
							loading="lazy"
							className="size-full object-cover"
						/>
					) : (
						t(`ui.assets.kind.${asset.kind}`)
					)}
				</span>
				<span className="truncate text-sm font-medium">
					<UntrustedText>{asset.name}</UntrustedText>
				</span>
				<span className="flex flex-wrap gap-1 text-xs text-muted-foreground">
					<span>
						{t(`ui.assets.origin.${asset.origin?.kind ?? "imported"}`)}
					</span>
					{asset.reusable === true && (
						<span className="rounded bg-primary/10 px-1 text-primary">
							{t("ui.assets.reusable_badge")}
						</span>
					)}
					{(asset.tags ?? []).map((tag) => (
						<span key={tag} className="rounded bg-muted px-1">
							<UntrustedText>{tag}</UntrustedText>
						</span>
					))}
				</span>
			</button>
		</li>
	);
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
			<h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
				{label}
			</h3>
			<div className="text-sm break-words whitespace-pre-wrap">{children}</div>
		</div>
	);
}

// The detail of one file: where it came from, the user's own notes, the projects that use it, and the few
// things a person can change. Every change is a call; the panel shows what the call answered.
export function AssetDetail({
	hash,
	onChanged,
	onClose,
}: {
	hash: string;
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
	const [said, setSaid] = useState<string | null>(null);

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

	async function addToProject(
		event: FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault();
		if (target === "") return;
		setFailure(null);
		setSaid(null);
		try {
			const result = await api().useAsset(target, hash);
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

	return (
		<aside
			aria-labelledby={`${id}-title`}
			className="flex flex-col gap-4 rounded-xl border bg-card p-4 text-card-foreground"
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
				<p role="status" className="text-sm text-muted-foreground">
					{said}
				</p>
			)}
			{asset !== null && (
				<>
					<Field label={t("ui.assets.detail.origin")}>
						<p data-testid="asset-origin">
							{t(`ui.assets.origin.${asset.origin?.kind ?? "imported"}`)}
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
						<p>
							{[
								t(`ui.assets.kind.${asset.kind}`),
								sizeText(asset.bytes, locale),
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
						<p className="text-xs text-muted-foreground">
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
						<h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
							{t("ui.assets.detail.tags")}
						</h3>
						<ul className="flex flex-wrap gap-2">
							{(asset.tags ?? []).map((name) => (
								<li
									key={name}
									className="flex items-center gap-1 rounded bg-muted px-2 py-1 text-sm"
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
						<h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
							{t("ui.assets.detail.uses")}
						</h3>
						{(asset.uses ?? []).length === 0 ? (
							<p className="text-sm text-muted-foreground">
								{t("ui.assets.detail.no_uses")}
							</p>
						) : (
							<ul
								className="flex flex-col gap-1 text-sm"
								data-testid="asset-uses"
							>
								{(asset.uses ?? []).map((use) => (
									<li key={use.project_id}>
										<Link
											href={`/projects/${use.project_id}`}
											className="underline"
										>
											<UntrustedText>{use.name}</UntrustedText>
										</Link>
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
											onChange={() => setTarget(project.id)}
										/>
										<UntrustedText>{project.name}</UntrustedText>
									</label>
								))}
							</fieldset>
							<div>
								<Button
									type="submit"
									variant="outline"
									disabled={target === ""}
								>
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

// One section of the library: the filters, the grid and the detail of the file that is open.
export function AssetLibrary({ section }: { section: Section }): ReactNode {
	const t = useT();
	const key = section === "visual" ? "visual-assets" : "video-assets";
	const [items, setItems] = useState<LibraryAsset[] | null>(null);
	const [total, setTotal] = useState(0);
	const [next, setNext] = useState<string | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [text, setText] = useState("");
	const [tag, setTag] = useState("");
	const [origin, setOrigin] = useState<OriginKind | "">("");
	const [reusable, setReusable] = useState(false);
	const [applied, setApplied] = useState<AssetQuery>({});
	const [open, setOpen] = useState<string | null>(null);
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

	function search(event: FormEvent<HTMLFormElement>): void {
		event.preventDefault();
		setApplied({
			...(text.trim() === "" ? {} : { text: text.trim() }),
			...(tag.trim() === "" ? {} : { tag: tag.trim() }),
			...(origin === "" ? {} : { origin }),
			...(reusable ? { reusable: true } : {}),
		});
	}

	function clear(): void {
		setText("");
		setTag("");
		setOrigin("");
		setReusable(false);
		setApplied({});
	}

	const filtered = Object.keys(applied).length > 0;

	return (
		<>
			<Head title={t(`ui.nav.section.${key}`)} />
			<div className="mx-auto flex max-w-6xl flex-col gap-6">
				<div className="flex flex-wrap items-start justify-between gap-3">
					<div>
						<h1 className="text-2xl font-semibold tracking-tight">
							{t(`ui.nav.section.${key}`)}
						</h1>
						<p className="mt-1 text-sm text-muted-foreground">
							{t(`ui.assets.intro.${section}`)}
						</p>
					</div>
					<div className="flex flex-wrap gap-2">
						<Button
							type="button"
							onClick={() =>
								setGenerating((was) => ({
									open: true,
									count: was.count + 1,
									kind: section === "visual" ? "image" : "voice",
								}))
							}
						>
							{t(`ui.generate.open.${section}`)}
						</Button>
						{section === "video" && (
							<Button
								type="button"
								variant="outline"
								onClick={() =>
									setGenerating((was) => ({
										open: true,
										count: was.count + 1,
										kind: "video",
									}))
								}
							>
								{t("ui.generate.open.clip")}
							</Button>
						)}
					</div>
				</div>
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
				<search>
					<form
						onSubmit={search}
						className="flex flex-wrap items-end gap-3 rounded-xl border bg-card p-4"
					>
						<div className="flex flex-col gap-1">
							<Label htmlFor={`filter-text-${section}`}>
								{t("ui.assets.filter.text")}
							</Label>
							<Input
								id={`filter-text-${section}`}
								autoComplete="off"
								value={text}
								onChange={(event) => setText(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor={`filter-tag-${section}`}>
								{t("ui.assets.filter.tag")}
							</Label>
							<Input
								id={`filter-tag-${section}`}
								autoComplete="off"
								value={tag}
								onChange={(event) => setTag(event.target.value)}
							/>
						</div>
						<fieldset className="flex flex-wrap gap-3">
							<legend className="mb-1 text-sm font-medium">
								{t("ui.assets.filter.origin")}
							</legend>
							{["", ...ORIGINS].map((kind) => (
								<label key={kind} className="flex items-center gap-1 text-sm">
									<input
										type="radio"
										name={`filter-origin-${section}`}
										className="accent-primary"
										checked={origin === kind}
										onChange={() => setOrigin(kind as OriginKind | "")}
									/>
									{kind === ""
										? t("ui.assets.filter.any_origin")
										: t(`ui.assets.origin.${kind}`)}
								</label>
							))}
						</fieldset>
						<label className="flex items-center gap-2 pb-2 text-sm">
							<input
								type="checkbox"
								className="accent-primary"
								checked={reusable}
								onChange={(event) => setReusable(event.target.checked)}
							/>
							{t("ui.assets.filter.reusable")}
						</label>
						<Button type="submit">{t("ui.assets.filter.apply")}</Button>
						<Button type="button" variant="outline" onClick={clear}>
							{t("ui.assets.filter.clear")}
						</Button>
					</form>
				</search>
				<FailureNote failure={failure} />
				<div className="grid gap-6 lg:grid-cols-[1fr_22rem]">
					<section aria-labelledby={`files-${section}`}>
						<h2
							id={`files-${section}`}
							className="text-sm text-muted-foreground"
						>
							{t("ui.assets.count", { count: String(total) })}
						</h2>
						{items !== null && items.length === 0 && (
							<p
								className="mt-4 text-sm text-muted-foreground"
								data-testid="asset-empty"
							>
								{filtered
									? t("ui.assets.empty_filtered")
									: t(`ui.assets.empty.${section}`)}
							</p>
						)}
						{items !== null && items.length > 0 && (
							<ul
								className="mt-3 grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-3"
								data-testid="asset-grid"
							>
								{items.map((asset) => (
									<AssetCard
										key={asset.hash}
										asset={asset}
										selected={open === asset.hash}
										onOpen={() => setOpen(asset.hash)}
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
							onChanged={() => void load(applied)}
							onClose={() => setOpen(null)}
						/>
					)}
				</div>
			</div>
		</>
	);
}
