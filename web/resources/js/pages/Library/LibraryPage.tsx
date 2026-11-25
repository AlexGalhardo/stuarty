import { Head } from "@inertiajs/react";
import {
	Brain,
	Check,
	FileText,
	Hourglass,
	MessageSquareText,
	Plus,
	RefreshCw,
	Sparkles,
	WandSparkles,
} from "lucide-react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import { EmptyState } from "@/components/EmptyState";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { FilterBar } from "@/components/FilterBar";
import { ListRow } from "@/components/ListRow";
import { LoadError } from "@/components/LoadError";
import { LoadingState } from "@/components/LoadingState";
import { Notice } from "@/components/Notice";
import { PageHeader, PageHeaderAction } from "@/components/PageHeader";
import { StatusBadge } from "@/components/StatusBadge";
import { TabBar } from "@/components/TabBar";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatAgo } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import { confirmDiscard } from "@/lib/unsaved";
import { HttpTransport } from "../../editor/api/http";
import {
	type LibraryClient,
	type LibraryEntry,
	type LibraryHit,
	type LibraryKind,
	libraryClient,
} from "./client";
import MemoryPane from "./MemoryPane";
import PromptPane from "./PromptPane";
import { groupEntries } from "./parts";
import SkillPane from "./SkillPane";

const EMPTY_ICON = {
	memory: Brain,
	skill: Sparkles,
	prompt: FileText,
} as const;

const ROW_ICON = {
	skill: WandSparkles,
	prompt: MessageSquareText,
	memory: Hourglass,
} as const;

const SECTION: { [kind in LibraryKind]: string } = {
	memory: "memory",
	skill: "skills",
	prompt: "prompts",
};

// A search waits for this pause after the last key, so the service is asked once per word.
const TYPING_PAUSE_MS = 300;

type Loaded =
	| { state: "loading" }
	| { state: "failed"; code: string }
	| { state: "ready"; entries: readonly LibraryEntry[] };

// What a row says under its name: the short words of what the service listed, never the long text of the
// file (that is in the detail). A skill says where it came from and whether it was read; a note, whether it
// is kept and how long ago it was written.
function useRowText(entry: LibraryEntry, hit: boolean): string {
	const t = useT();
	const locale = useLocale();
	if (hit) return String((entry as LibraryHit).snippet);
	if (entry.state === "invalid") return entry.error ?? "";
	if (entry.kind === "skill") {
		return t("ui.library.row.skill", {
			origin: t(`ui.library.origin.${entry.origin ?? "user"}`),
			review: t(
				entry.reviewed
					? "ui.library.row.reviewed"
					: "ui.library.row.unreviewed",
			),
		});
	}
	if (entry.kind === "memory") {
		return t(entry.kept ? "ui.library.row.kept" : "ui.library.row.pending", {
			ago: formatAgo(entry.updated_at, locale),
		});
	}
	return entry.description;
}

// A section of the library (SDD-018 section 14): the list with a search box, and beside it the entry that is
// open. The files are the truth; the page reads and writes them through the API.
export default function LibraryPage({
	kind,
}: {
	kind: LibraryKind;
}): ReactNode {
	const t = useT();
	const { privileged } = useSession();
	const client: LibraryClient = useMemo(
		() => libraryClient(new HttpTransport("/api/v1")),
		[],
	);
	const [loaded, setLoaded] = useState<Loaded>({ state: "loading" });
	const [selected, setSelected] = useState<string | null>(null);
	const [creating, setCreating] = useState(false);
	// A new key makes a pane of its own when the person picks another entry; saving keeps the pane, so its notice stays.
	const [paneKey, setPaneKey] = useState(0);
	const [query, setQuery] = useState("");
	const [hits, setHits] = useState<readonly LibraryHit[] | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [notice, setNotice] = useState<string | null>(null);
	const [tab, setTab] = useState("all");
	const [busyRow, setBusyRow] = useState<string | null>(null);

	const reload = useCallback(async (): Promise<void> => {
		try {
			const result = await client.list(kind);
			setLoaded({ state: "ready", entries: result.entries });
		} catch (error) {
			setLoaded({
				state: "failed",
				code: (error as { code?: string }).code ?? "E_GENERIC",
			});
		}
	}, [client, kind]);

	useEffect(() => {
		void reload();
	}, [reload]);

	const first = useRef(true);
	useEffect(() => {
		if (first.current) {
			first.current = false;
			return;
		}
		const word = query.trim();
		if (word === "") {
			setHits(null);
			return;
		}
		let live = true;
		const timer = window.setTimeout(() => {
			setFailure(null);
			client
				.search(kind, word)
				.then((result) => {
					if (live) setHits(result.hits);
				})
				.catch((error: unknown) => {
					if (live) setFailure(failureOf(error));
				});
		}, TYPING_PAUSE_MS);
		return () => {
			live = false;
			window.clearTimeout(timer);
		};
	}, [query, client, kind]);

	async function rescan(): Promise<void> {
		setFailure(null);
		try {
			const result = await client.reindex();
			setNotice(
				t("ui.library.rescanned", {
					memory: result.memory,
					skills: result.skills,
					prompts: result.prompts,
					invalid: result.invalid,
				}),
			);
			await reload();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	function changed(name: string | null): void {
		// The results of an earlier search may name what has just changed: show the list again.
		setHits(null);
		setQuery("");
		setCreating(false);
		setSelected(name);
		void reload();
	}

	// "Keep this note" from a row: the note is read, then written again with the mark that a person kept it. The
	// service decides who may keep it; a refusal is shown as it came.
	async function keepNote(name: string): Promise<void> {
		setBusyRow(name);
		setFailure(null);
		try {
			const note = await client.getMemory(name);
			await client.writeMemory({
				name,
				description: note.entry.description,
				scope:
					note.entry.scope === undefined || note.entry.scope === "workspace"
						? undefined
						: note.entry.scope,
				tags: note.entry.tags ?? [],
				body: note.body,
				keep: true,
			});
			if (selected === name) setPaneKey((k) => k + 1);
			await reload();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusyRow(null);
		}
	}

	async function removeNote(name: string): Promise<void> {
		setBusyRow(name);
		setFailure(null);
		try {
			await client.removeMemory(name);
			if (selected === name) changed(null);
			else await reload();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusyRow(null);
		}
	}

	function pick(name: string): void {
		if (!creating && selected === name) return;
		if (!confirmDiscard(t("ui.unsaved.leave"))) return;
		setCreating(false);
		setSelected(name);
		setPaneKey((k) => k + 1);
	}

	const all: readonly LibraryEntry[] =
		loaded.state === "ready" ? loaded.entries : [];
	const isMemory = kind === "memory";
	const pendingCount = isMemory
		? all.filter((e) => e.state === "ok" && !e.kept).length
		: 0;
	const keptCount = isMemory
		? all.filter((e) => e.state === "ok" && e.kept).length
		: 0;
	let rows: readonly LibraryEntry[] = hits ?? all;
	if (isMemory && tab === "pending") {
		rows = rows.filter((e) => e.state === "ok" && !e.kept);
	}
	if (isMemory && tab === "kept") {
		rows = rows.filter((e) => e.state === "ok" && e.kept);
	}
	const section = SECTION[kind];
	// Memory shows the notes that wait for a person first (the service says which are kept); the other lists keep
	// the order of the service.
	const groups = groupEntries(kind, rows, {
		pending: t("ui.library.group.pending"),
		kept: t("ui.library.group.kept"),
		invalid: t("ui.library.group.invalid"),
	});
	const pane = creating || selected !== null;
	const name = creating ? null : selected;
	const split = pane || !isMemory;
	let layout = "flex flex-col gap-6";
	if (split) {
		layout =
			kind === "prompt"
				? "grid items-start gap-6 lg:grid-cols-[minmax(14rem,20rem)_minmax(0,1fr)]"
				: "grid items-start gap-6 lg:grid-cols-[minmax(16rem,24rem)_minmax(0,1fr)]";
	}

	return (
		<>
			<Head title={t(`ui.nav.section.${section}`)} />
			<section
				aria-label={t(`ui.nav.section.${section}`)}
				className="mx-auto flex max-w-7xl flex-col gap-6"
				data-testid={`library-${kind}`}
			>
				<PageHeader
					title={t(`ui.nav.section.${section}`)}
					description={t(`ui.library.intro.${kind}`)}
					actions={
						<>
							<PageHeaderAction
								type="button"
								variant="outline"
								onClick={() => void rescan()}
							>
								<RefreshCw aria-hidden="true" />
								{t("ui.library.rescan")}
							</PageHeaderAction>
							<PageHeaderAction
								type="button"
								onClick={() => {
									if (!confirmDiscard(t("ui.unsaved.leave"))) return;
									setCreating(true);
									setSelected(null);
									setPaneKey((k) => k + 1);
								}}
							>
								<Plus aria-hidden="true" />
								{t(`ui.library.new.${kind}`)}
							</PageHeaderAction>
						</>
					}
				/>
				{notice !== null && <Notice tone="info" role="status" title={notice} />}
				<FailureNote failure={failure} />
				{isMemory && pendingCount > 0 && (
					<Notice
						tone="warning"
						title={t("ui.library.memory.waiting", { count: pendingCount })}
						data-testid="memory-waiting"
					>
						{t("ui.library.memory.waiting_hint")}
					</Notice>
				)}
				{isMemory && (
					<TabBar
						label={t("ui.library.list.memory")}
						value={tab}
						onChange={setTab}
						tabs={[
							{ value: "all", label: t("ui.library.tabs.all") },
							{
								value: "pending",
								label: t("ui.library.tabs.pending"),
								count: pendingCount,
							},
							{
								value: "kept",
								label: t("ui.library.tabs.kept"),
								count: keptCount,
							},
						]}
					/>
				)}
				<div className={layout}>
					<div className="flex min-w-0 flex-col gap-3">
						<FilterBar label={t("ui.library.search_bar")}>
							<div className="flex min-w-0 flex-1 flex-col gap-1">
								<Label htmlFor="library-query">{t("ui.library.search")}</Label>
								<Input
									id="library-query"
									type="search"
									autoComplete="off"
									placeholder={t(`ui.library.search_hint.${kind}`)}
									value={query}
									onChange={(e) => setQuery(e.target.value)}
								/>
							</div>
						</FilterBar>
						<nav aria-label={t(`ui.library.list.${kind}`)}>
							{loaded.state === "loading" && (
								<LoadingState message={t("ui.library.loading")} />
							)}
							{loaded.state === "failed" && (
								<LoadError
									failure={{ code: loaded.code, message: "" }}
									onRetry={() => {
										setLoaded({ state: "loading" });
										void reload();
									}}
								/>
							)}
							{loaded.state === "ready" &&
								rows.length === 0 &&
								(hits !== null || tab !== "all") && (
									<p className="text-sm text-text-muted">
										{t("ui.library.no_match")}
									</p>
								)}
							{loaded.state === "ready" &&
								rows.length === 0 &&
								hits === null &&
								tab === "all" && (
									<EmptyState
										icon={EMPTY_ICON[kind]}
										level={2}
										title={t("ui.library.empty")}
										description={t(`ui.library.empty_text.${kind}`)}
										steps={[1, 2, 3].map((n) => ({
											title: t(`ui.library.empty_steps.${kind}.${n}`),
										}))}
									/>
								)}
							{rows.length > 0 && (
								<div className="flex flex-col divide-y divide-border rounded-lg border border-border bg-surface">
									{groups.map((group) => (
										<ul
											key={group.key}
											aria-label={group.label ?? undefined}
											data-group={group.key}
											className="flex flex-col divide-y divide-border"
										>
											{group.entries.map((entry) => (
												<EntryRow
													key={entry.name}
													entry={entry}
													hit={hits !== null}
													selected={!creating && selected === entry.name}
													busy={busyRow === entry.name}
													privileged={privileged}
													onPick={() => pick(entry.name)}
													onKeep={() => void keepNote(entry.name)}
													onRemove={() => void removeNote(entry.name)}
												/>
											))}
										</ul>
									))}
								</div>
							)}
							{isMemory && pendingCount > 0 && !privileged && (
								<PersonOnlyNote id="keep-person-only-list" />
							)}
						</nav>
					</div>
					{split && (
						<div className="min-w-0">
							{!pane && (
								<p className="text-sm text-text-muted">
									{t("ui.library.pick")}
								</p>
							)}
							{pane && kind === "memory" && (
								<MemoryPane
									key={paneKey}
									client={client}
									name={name}
									onChanged={changed}
								/>
							)}
							{pane && kind === "prompt" && (
								<PromptPane
									key={paneKey}
									client={client}
									name={name}
									onChanged={changed}
								/>
							)}
							{pane && kind === "skill" && (
								<SkillPane
									key={paneKey}
									client={client}
									name={name}
									onChanged={changed}
								/>
							)}
						</div>
					)}
				</div>
				{kind === "skill" && (
					<SkillTools
						client={client}
						onImported={(name) => {
							setPaneKey((k) => k + 1);
							setSelected(name);
							setCreating(false);
							void reload();
						}}
					/>
				)}
			</section>
		</>
	);
}

// One row of a list: what the service listed, in short words, and the actions a note that waits needs.
function EntryRow({
	entry,
	hit,
	selected,
	busy,
	privileged,
	onPick,
	onKeep,
	onRemove,
}: {
	entry: LibraryEntry;
	hit: boolean;
	selected: boolean;
	busy: boolean;
	privileged: boolean;
	onPick(): void;
	onKeep(): void;
	onRemove(): void;
}): ReactNode {
	const t = useT();
	const text = useRowText(entry, hit);
	const memory = entry.kind === "memory" && entry.state === "ok";
	const waiting = memory && !entry.kept;
	let Leading = ROW_ICON[entry.kind];
	if (memory) Leading = waiting ? Hourglass : Check;
	let tone = "bg-surface-hover text-icon";
	if (waiting) tone = "bg-warning-soft text-warning-text";
	else if (memory) tone = "bg-success-soft text-success-text";
	return (
		<ListRow
			selected={selected}
			className={memory ? "py-4" : undefined}
			leading={
				<span
					className={`flex size-9 items-center justify-center rounded-md ${tone}`}
				>
					<Leading aria-hidden="true" className="size-4" />
				</span>
			}
			title={
				<button
					type="button"
					data-entry={entry.name}
					className="block w-full truncate text-start focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
					onClick={onPick}
				>
					<UntrustedText>
						{memory && entry.description !== ""
							? entry.description
							: entry.name}
					</UntrustedText>
				</button>
			}
			description={text}
			trailing={
				<>
					{entry.state === "invalid" && (
						<StatusBadge status="danger">
							{t("ui.library.badge.invalid_short")}
						</StatusBadge>
					)}
					{entry.kind === "skill" && entry.state === "ok" && (
						<span className="text-sm text-text-muted">
							{t(
								entry.reviewed
									? "ui.library.state.available"
									: "ui.library.state.blocked",
							)}
						</span>
					)}
					{memory && !waiting && (
						<StatusBadge status="success">
							{t("ui.library.badge.kept_short")}
						</StatusBadge>
					)}
					{waiting && (
						<>
							<Button
								type="button"
								variant="ghost"
								disabled={busy}
								onClick={onRemove}
							>
								{t("ui.library.action.remove")}
							</Button>
							<Button
								type="button"
								disabled={busy || !privileged}
								aria-describedby={
									privileged ? undefined : "keep-person-only-list"
								}
								onClick={onKeep}
							>
								<Check aria-hidden="true" />
								{t("ui.library.memory.keep")}
							</Button>
						</>
					)}
				</>
			}
		/>
	);
}

// The import and the export of skills. Stuarty opens no connection to fetch a skill: the path is a folder or an
// archive that is already on the machine of the service.
function SkillTools({
	client,
	onImported,
}: {
	client: LibraryClient;
	onImported: (name: string) => void;
}): ReactNode {
	const t = useT();
	const [path, setPath] = useState("");
	const [source, setSource] = useState("");
	const [replace, setReplace] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [notice, setNotice] = useState<string | null>(null);
	const [busy, setBusy] = useState(false);

	async function importIt(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		setNotice(null);
		try {
			const result = await client.importSkill({
				path: path.trim(),
				source: source.trim() === "" ? undefined : source.trim(),
				replace,
			});
			setNotice(t("ui.library.import.done", { name: result.entry.name }));
			onImported(result.entry.name);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	async function exportIt(): Promise<void> {
		setBusy(true);
		setFailure(null);
		setNotice(null);
		try {
			const result = await client.exportSkills([]);
			setNotice(
				result.names.length === 0
					? t("ui.library.export.none")
					: t("ui.library.export.done", {
							count: result.names.length,
							path: result.path ?? "",
						}),
			);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<details className="rounded-lg border px-3 py-2">
			<summary className="cursor-pointer text-sm font-medium">
				{t("ui.library.import.heading")}
			</summary>
			<form className="mt-3 flex flex-col gap-3" onSubmit={importIt}>
				<p className="text-sm text-text-muted">{t("ui.library.import.hint")}</p>
				<div className="flex flex-col gap-1">
					<Label htmlFor="import-path">{t("ui.library.import.path")}</Label>
					<Input
						id="import-path"
						value={path}
						required
						onChange={(e) => setPath(e.target.value)}
					/>
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor="import-source">{t("ui.library.import.source")}</Label>
					<Input
						id="import-source"
						value={source}
						onChange={(e) => setSource(e.target.value)}
					/>
				</div>
				<div className="flex items-center gap-2">
					<Checkbox
						id="import-replace"
						checked={replace}
						onCheckedChange={(value) => setReplace(value === true)}
					/>
					<Label htmlFor="import-replace">
						{t("ui.library.import.replace")}
					</Label>
				</div>
				<div className="flex flex-wrap gap-2">
					<Button type="submit" disabled={busy}>
						{t("ui.library.import.button")}
					</Button>
					<Button
						type="button"
						variant="outline"
						disabled={busy}
						onClick={() => void exportIt()}
					>
						{t("ui.library.export.button")}
					</Button>
				</div>
				{notice !== null && (
					<p role="status" className="text-sm">
						{notice}
					</p>
				)}
				<FailureNote failure={failure} />
			</form>
		</details>
	);
}
