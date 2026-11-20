import { ArrowLeft, CircleAlert, Search } from "lucide-react";
import {
	type KeyboardEvent,
	type ReactNode,
	useEffect,
	useId,
	useMemo,
	useState,
} from "react";

import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { type Translate, useLocale, useT } from "@/lib/i18n";
import { ApiError } from "../../api/errors";
import type { CommandDetail, EditResult } from "../../api/types";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import { errorHeadline } from "../../ui/errorHeadline";
import ErrorBody from "../ErrorBody";
import { hasMod } from "../keymap";
import { SchemaForm } from "../schemaForm/SchemaForm";
import { useCanEdit } from "../useActions";
import BatchTray from "./BatchTray";
import {
	addToBatch,
	type BatchEntry,
	type BatchOperation,
	removeFromBatch,
	sendBatch,
} from "./batch";
import type { CommandArgs, FormSlot } from "./formSlot";
import {
	commandRow,
	matchRange,
	type PaletteRow,
	usePaletteSearch,
	useRegistry,
} from "./usePaletteSearch";

const PAGE = 8;
const PREVIEW_LINES = 12;

type Detail =
	| { status: "loading"; name: string }
	| { status: "ready"; command: CommandDetail }
	| { status: "failed"; name: string; error: ApiError };

function ErrorBlock({ error }: { error: ApiError }): ReactNode {
	const t = useT();
	return (
		<div
			role="alert"
			className="flex gap-2 rounded-md border border-destructive p-2 text-sm"
		>
			<CircleAlert
				aria-hidden="true"
				className="mt-0.5 size-4 shrink-0 text-destructive"
			/>
			<div className="min-w-0">
				<p className="font-semibold">{errorHeadline(t, error.code)}</p>
				<ErrorBody
					error={error}
					hintPrefix={t("ui.editor.palette.how_to_fix")}
				/>
				<p className="font-mono text-xs text-muted-foreground">{error.code}</p>
			</div>
		</div>
	);
}

function Title({ row, query }: { row: PaletteRow; query: string }): ReactNode {
	const lang = row.translated ? undefined : "en";
	const range = row.translated ? matchRange(row.title, query) : null;
	if (range === null) return <span lang={lang}>{row.title}</span>;
	const [from, to] = range;
	return (
		<span lang={lang}>
			{row.title.slice(0, from)}
			<strong>{row.title.slice(from, to)}</strong>
			{row.title.slice(to)}
		</span>
	);
}

// The core's answer to a dry run, as received: the description, then one line
// per change of the diff.
function Preview({ result }: { result: EditResult }): ReactNode {
	const t = useT();
	const [all, setAll] = useState(false);
	const lines = [
		result.description,
		...result.diff.map((change) => JSON.stringify(change)),
	];
	const shown = all ? lines : lines.slice(0, PREVIEW_LINES);
	return (
		<section aria-label={t("ui.editor.palette.preview_result")}>
			<pre
				lang="en"
				className="overflow-auto rounded-md bg-muted p-2 font-mono text-xs"
			>
				<UntrustedText>{shown.join("\n")}</UntrustedText>
			</pre>
			{!all && lines.length > PREVIEW_LINES && (
				<Button
					type="button"
					variant="link"
					size="sm"
					onClick={() => setAll(true)}
				>
					{t("ui.editor.palette.show_all")}
				</Button>
			)}
		</section>
	);
}

function titleOf(t: Translate, command: CommandDetail): string {
	return commandRow(t, command).title;
}

// The command palette of SDD-012 section 9.2 and docs/design/alpha-editor.md
// section 2: the list comes from the live registry, a row opens the form slot,
// and the commands of the batch go to the core in one `edit()` call. Run and
// Preview in the form send the batch followed by the command being edited.
export default function Palette({
	open,
	onOpenChange,
	FormView = SchemaForm,
}: {
	open: boolean;
	onOpenChange(open: boolean): void;
	FormView?: FormSlot;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const runtime = useRuntime();
	const canEdit = useCanEdit();
	const revision = useDocumentState((s) => s.revision);
	const listId = useId();
	const { state: registry, reload } = useRegistry(open);

	const [query, setQuery] = useState("");
	const [active, setActive] = useState(0);
	const [detail, setDetail] = useState<Detail | null>(null);
	const [args, setArgs] = useState<CommandArgs | null>({});
	const [fills, setFills] = useState(0);
	const [batch, setBatch] = useState<BatchEntry[]>([]);
	const [preview, setPreview] = useState<EditResult | null>(null);
	const [error, setError] = useState<ApiError | null>(null);
	const [busy, setBusy] = useState<"preview" | "run" | null>(null);

	const rows = usePaletteSearch(t, locale, registry, query);
	const activeRow = rows[Math.min(active, rows.length - 1)];
	// The combobox points at its listbox only while the listbox is in the page: while the registry loads, or
	// when nothing matches, an aria-controls to a missing id is an invalid value (axe aria-valid-attr-value).
	const listShown = registry.status === "ready" && rows.length > 0;
	const count = useMemo(
		() =>
			registry.status === "ready"
				? registry.commands.filter((c) => c.internal !== true).length
				: 0,
		[registry],
	);

	useEffect(() => {
		if (!open) {
			setDetail(null);
			setQuery("");
			setPreview(null);
			setError(null);
		}
	}, [open]);

	function openForm(row: PaletteRow): void {
		const name = row.command.name;
		setDetail({ status: "loading", name });
		setArgs({});
		setPreview(null);
		setError(null);
		runtime.client
			.getCommand(name)
			.then((result) => {
				if (result.command === undefined) return;
				setDetail({ status: "ready", command: result.command });
			})
			.catch((failure: unknown) => {
				if (failure instanceof ApiError)
					setDetail({ status: "failed", name, error: failure });
			});
	}

	function back(): void {
		setDetail(null);
		setPreview(null);
		setError(null);
	}

	const current: BatchOperation | null =
		detail?.status === "ready" && args !== null
			? { op: detail.command.name, args }
			: null;
	const sendable = canEdit && revision !== null && busy === null;

	async function submit(
		operations: readonly BatchOperation[],
		dryRun: boolean,
	): Promise<void> {
		// The revision at the moment of sending, not the one the form opened at.
		const base = runtime.documents.getState().revision;
		if (operations.length === 0 || base === null) return;
		setBusy(dryRun ? "preview" : "run");
		setError(null);
		const outcome = await sendBatch(runtime.edits, base, operations, dryRun);
		setBusy(null);
		if (!outcome.ok) {
			setPreview(null);
			setError(outcome.error);
			return;
		}
		if (dryRun || !outcome.result.applied) {
			setPreview(outcome.result);
			return;
		}
		runtime.noticeWriter.push({
			kind: "info",
			message: outcome.result.description,
		});
		setBatch([]);
		setDetail(null);
		onOpenChange(false);
	}

	function formOperations(): BatchOperation[] {
		return [
			...batch.map((entry) => entry.operation),
			...(current === null ? [] : [current]),
		];
	}

	function onListKey(event: KeyboardEvent): void {
		if (rows.length === 0) return;
		const index = Math.min(active, rows.length - 1);
		const to = (next: number): void => {
			event.preventDefault();
			setActive(((next % rows.length) + rows.length) % rows.length);
		};
		if (event.key === "ArrowDown") to(index + 1);
		else if (event.key === "ArrowUp") to(index - 1);
		else if (event.key === "PageDown")
			to(Math.min(index + PAGE, rows.length - 1));
		else if (event.key === "PageUp") to(Math.max(index - PAGE, 0));
		else if (event.key === "Enter" && activeRow !== undefined) {
			event.preventDefault();
			openForm(activeRow);
		}
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				showCloseButton={false}
				className="top-[20%] max-h-[calc(100vh-20vh-24px)] translate-y-0 gap-0 overflow-hidden p-0 sm:max-w-[640px]"
				aria-describedby={undefined}
				onEscapeKeyDown={(event) => {
					if (detail !== null) {
						event.preventDefault();
						back();
					}
				}}
			>
				<DialogTitle className="sr-only">
					{t("ui.editor.palette.title")}
				</DialogTitle>
				{detail === null ? (
					<>
						<div className="relative border-b border-border p-3">
							<Search
								aria-hidden="true"
								className="absolute top-1/2 left-5 size-4 -translate-y-1/2 text-muted-foreground"
							/>
							<Input
								role="combobox"
								aria-expanded={listShown}
								aria-controls={listShown ? listId : undefined}
								aria-activedescendant={
									listShown && activeRow
										? `${listId}-${activeRow.command.name}`
										: undefined
								}
								aria-label={t("ui.editor.palette.search")}
								placeholder={t("ui.editor.palette.search")}
								className="h-10 pl-8"
								value={query}
								onChange={(event) => {
									setQuery(event.target.value);
									setActive(0);
								}}
								onKeyDown={onListKey}
							/>
						</div>
						{registry.status === "loading" && (
							<div className="flex flex-col gap-1 p-3" aria-busy="true">
								<Skeleton className="h-8" />
								<Skeleton className="h-8" />
								<Skeleton className="h-8" />
							</div>
						)}
						{registry.status === "failed" && (
							<div className="flex flex-col items-start gap-2 p-3">
								<ErrorBlock error={registry.error} />
								<Button
									type="button"
									size="sm"
									variant="outline"
									onClick={reload}
								>
									{t("ui.editor.palette.retry")}
								</Button>
							</div>
						)}
						{registry.status === "ready" && rows.length === 0 && (
							<div className="p-6 text-center text-sm">
								<p className="font-medium">{t("ui.editor.palette.no_match")}</p>
								<p className="text-muted-foreground">
									{t("ui.editor.palette.no_match_hint")}
								</p>
							</div>
						)}
						{registry.status === "ready" && rows.length > 0 && (
							<div
								id={listId}
								role="listbox"
								aria-label={t("ui.editor.palette.title")}
								className="max-h-80 overflow-y-auto"
							>
								{rows.map((row, index) => (
									<div
										key={row.command.name}
										id={`${listId}-${row.command.name}`}
										role="option"
										tabIndex={-1}
										aria-selected={row === activeRow}
										data-command={row.command.name}
										className="grid h-10 cursor-pointer grid-cols-[1fr_auto_auto] items-center gap-3 px-3 text-sm aria-selected:bg-accent aria-selected:shadow-[inset_2px_0_0_var(--ring)]"
										onMouseMove={() => setActive(index)}
										onClick={() => openForm(row)}
										onKeyDown={() => undefined}
									>
										<Title row={row} query={query} />
										<span className="font-mono text-xs text-muted-foreground">
											{row.command.name}
										</span>
										<Badge variant="outline">{row.command.scope}</Badge>
									</div>
								))}
							</div>
						)}
						{error !== null && (
							<div className="px-3 py-2">
								<ErrorBlock error={error} />
							</div>
						)}
						{preview !== null && (
							<div className="px-3 py-2">
								<Preview result={preview} />
							</div>
						)}
						<div className="flex h-8 items-center justify-between border-t border-border px-3 text-xs text-muted-foreground">
							<span>
								{t("ui.editor.palette.count", { count })}
								{" · "}
								{t("ui.editor.palette.hints")}
							</span>
							<span>
								{t("ui.editor.palette.batch.count", { count: batch.length })}
							</span>
						</div>
					</>
				) : (
					<section
						aria-label={t("ui.editor.palette.form")}
						className="flex flex-col gap-3 overflow-y-auto p-3"
						onKeyDown={(event) => {
							if (event.key === "Enter" && hasMod(event)) {
								event.preventDefault();
								if (sendable && current !== null)
									void submit(formOperations(), false);
							}
						}}
					>
						<div className="flex items-center gap-2">
							<Button
								type="button"
								size="icon-sm"
								variant="ghost"
								aria-label={t("ui.editor.palette.back")}
								onClick={back}
							>
								<ArrowLeft aria-hidden="true" />
							</Button>
							<span className="text-lg font-semibold">
								{detail.status === "ready"
									? titleOf(t, detail.command)
									: detail.name}
							</span>
							<span className="font-mono text-xs text-muted-foreground">
								{detail.status === "ready" ? detail.command.name : detail.name}
							</span>
							{detail.status === "ready" && (
								<Badge variant="outline">{detail.command.scope}</Badge>
							)}
						</div>
						{detail.status === "loading" && (
							<Skeleton aria-busy="true" className="h-24" />
						)}
						{detail.status === "failed" && <ErrorBlock error={detail.error} />}
						{detail.status === "ready" && (
							<>
								<p lang="en" className="text-sm text-muted-foreground">
									{detail.command.summary}
								</p>
								{error !== null &&
									!FormView.resolvesPath?.(detail.command, error) && (
										<ErrorBlock error={error} />
									)}
								<FormView
									command={detail.command}
									value={args ?? {}}
									revision={fills}
									onChange={setArgs}
									error={error}
								/>
								{preview !== null && <Preview result={preview} />}
								{!canEdit && (
									<p className="text-xs text-muted-foreground">
										{t("ui.editor.palette.read_only")}
									</p>
								)}
								<div className="flex flex-wrap items-center gap-2">
									<Button
										type="button"
										variant="ghost"
										onClick={() => {
											setArgs(detail.command.example as CommandArgs);
											setFills((n) => n + 1);
										}}
									>
										{t("ui.editor.palette.fill_example")}
									</Button>
									<div className="ml-auto flex gap-2">
										<Button
											type="button"
											variant="secondary"
											disabled={!canEdit || current === null}
											onClick={() => {
												if (current === null) return;
												setBatch((b) => addToBatch(b, current));
												back();
											}}
										>
											{t("ui.editor.palette.add_batch")}
										</Button>
										<Button
											type="button"
											variant="outline"
											disabled={!sendable || current === null}
											aria-busy={busy === "preview"}
											onClick={() => void submit(formOperations(), true)}
										>
											{t("ui.editor.palette.preview")}
										</Button>
										<Button
											type="button"
											disabled={!sendable || current === null}
											aria-busy={busy === "run"}
											onClick={() => void submit(formOperations(), false)}
										>
											{t("ui.editor.palette.run")}
										</Button>
									</div>
								</div>
							</>
						)}
					</section>
				)}
				<BatchTray
					batch={batch}
					disabled={!sendable}
					busy={busy !== null}
					onRemove={(id) => setBatch((b) => removeFromBatch(b, id))}
					onClear={() => setBatch([])}
					onPreview={() =>
						void submit(
							batch.map((e) => e.operation),
							true,
						)
					}
					onRun={() =>
						void submit(
							batch.map((e) => e.operation),
							false,
						)
					}
				/>
			</DialogContent>
		</Dialog>
	);
}
