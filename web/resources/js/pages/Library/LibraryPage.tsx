import { Head } from "@inertiajs/react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
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
import { EntryBadges } from "./parts";
import SkillPane from "./SkillPane";

const SECTION: { [kind in LibraryKind]: string } = {
	memory: "memory",
	skill: "skills",
	prompt: "prompts",
};

type Loaded =
	| { state: "loading" }
	| { state: "failed" }
	| { state: "ready"; entries: readonly LibraryEntry[] };

// A section of the library (SDD-018 section 14): the list with a search box, an editor with a preview, and for
// skills the import and the export. The files are the truth; the page reads and writes them through the API.
export default function LibraryPage({
	kind,
}: {
	kind: LibraryKind;
}): ReactNode {
	const t = useT();
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

	const reload = useCallback(async (): Promise<void> => {
		try {
			const result = await client.list(kind);
			setLoaded({ state: "ready", entries: result.entries });
		} catch {
			setLoaded({ state: "failed" });
		}
	}, [client, kind]);

	useEffect(() => {
		void reload();
	}, [reload]);

	async function search(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setFailure(null);
		if (query.trim() === "") {
			setHits(null);
			return;
		}
		try {
			const result = await client.search(kind, query.trim());
			setHits(result.hits);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

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

	const rows: readonly LibraryEntry[] =
		hits ?? (loaded.state === "ready" ? loaded.entries : []);
	const section = SECTION[kind];
	const pane = creating || selected !== null;
	const name = creating ? null : selected;

	return (
		<>
			<Head title={t(`ui.nav.section.${section}`)} />
			<section
				aria-labelledby="library-title"
				className="flex flex-col gap-4"
				data-testid={`library-${kind}`}
			>
				<header className="flex flex-col gap-1">
					<h1 id="library-title" className="text-2xl font-semibold">
						{t(`ui.nav.section.${section}`)}
					</h1>
					<p className="text-sm text-muted-foreground">
						{t(`ui.library.intro.${kind}`)}
					</p>
				</header>
				<div className="flex flex-wrap items-end gap-2">
					<search className="flex-1">
						<form className="flex flex-wrap items-end gap-2" onSubmit={search}>
							<div className="flex min-w-48 flex-1 flex-col gap-1">
								<Label htmlFor="library-query">{t("ui.library.search")}</Label>
								<Input
									id="library-query"
									type="search"
									value={query}
									onChange={(e) => {
										setQuery(e.target.value);
										if (e.target.value === "") setHits(null);
									}}
								/>
							</div>
							<Button type="submit" variant="outline">
								{t("ui.library.search_button")}
							</Button>
						</form>
					</search>
					<Button
						type="button"
						onClick={() => {
							if (!confirmDiscard(t("ui.unsaved.leave"))) return;
							setCreating(true);
							setSelected(null);
							setPaneKey((k) => k + 1);
						}}
					>
						{t("ui.library.new")}
					</Button>
					<Button type="button" variant="outline" onClick={() => void rescan()}>
						{t("ui.library.rescan")}
					</Button>
				</div>
				{notice !== null && (
					<p role="status" className="text-sm">
						{notice}
					</p>
				)}
				<FailureNote failure={failure} />
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
				<div className="grid gap-4 md:grid-cols-[minmax(14rem,22rem)_1fr]">
					<nav aria-label={t(`ui.library.list.${kind}`)}>
						{loaded.state === "loading" && (
							<p className="text-sm text-muted-foreground">
								{t("ui.library.loading")}
							</p>
						)}
						{loaded.state === "failed" && (
							<p role="alert" className="text-sm text-destructive">
								{t("ui.library.load_failed")}
							</p>
						)}
						{loaded.state === "ready" && rows.length === 0 && (
							<p className="text-sm text-muted-foreground">
								{hits === null
									? t("ui.library.empty")
									: t("ui.library.no_match")}
							</p>
						)}
						<ul className="flex flex-col gap-1">
							{rows.map((entry) => (
								<li key={entry.name}>
									<button
										type="button"
										data-entry={entry.name}
										aria-current={
											!creating && selected === entry.name ? "true" : undefined
										}
										className="flex w-full flex-col gap-1 rounded-lg border px-3 py-2 text-left hover:bg-muted aria-[current=true]:border-ring aria-[current=true]:bg-muted"
										onClick={() => {
											if (!creating && selected === entry.name) return;
											if (!confirmDiscard(t("ui.unsaved.leave"))) return;
											setCreating(false);
											setSelected(entry.name);
											setPaneKey((k) => k + 1);
										}}
									>
										<span className="flex flex-wrap items-center gap-2">
											<span className="font-medium">{entry.name}</span>
											<EntryBadges entry={entry} />
										</span>
										<span className="text-sm text-muted-foreground">
											<UntrustedText>
												{"snippet" in entry
													? String((entry as LibraryHit).snippet)
													: entry.description}
											</UntrustedText>
										</span>
										{entry.state === "invalid" && entry.error !== undefined && (
											<span className="text-xs text-destructive" lang="en">
												<UntrustedText>{entry.error}</UntrustedText>
											</span>
										)}
									</button>
								</li>
							))}
						</ul>
					</nav>
					<div>
						{!pane && (
							<p className="text-sm text-muted-foreground">
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
				</div>
			</section>
		</>
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
				<p className="text-sm text-muted-foreground">
					{t("ui.library.import.hint")}
				</p>
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
