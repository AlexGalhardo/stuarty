import { Film, Plus } from "lucide-react";
import {
	type ReactNode,
	useEffect,
	useMemo,
	useState,
	useSyncExternalStore,
} from "react";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import {
	canDropPaths,
	canPickFiles,
	cleanPaths,
	getDesktopBridge,
	subscribeDesktopBridge,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import { useCanEdit } from "../commands/useActions";
import RecordDialog from "../record/RecordDialog";
import { useStore } from "../state/createStore";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import EmptyState from "../ui/EmptyState";
import { errorHeadline } from "../ui/errorHeadline";
import AssetCard, { type AssetDragStart } from "./AssetCard";
import {
	createImportQueue,
	OPEN_IMPORT_EVENT,
	parsePastedPaths,
} from "./importPaths";
import { PanelTitle } from "./PanelFrame";
import TrashGroup from "./TrashGroup";

// The media library of SDD-012 section 13: import by path, cards with their
// state and progress from events, and "Add to timeline". A browser page cannot
// see the path of a dropped file, so a drop only explains that; the desktop
// shell (window.vukemDesktop, feature-detected) adds "Choose files" and drops
// that carry absolute paths, which go to the same importPaths.
export default function Library({
	onAssetDragStart,
}: {
	onAssetDragStart?: AssetDragStart;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const runtime = useRuntime();
	// A library still waiting for the project is loading, not empty.
	const loading = useDocumentState((s) => s.status === "loading");
	const entries = useStore(runtime.library, (s) => s.entries);
	const dismissed = useStore(runtime.library, (s) => s.dismissed);
	const importError = useStore(runtime.library, (s) => s.importError);
	const [text, setText] = useState("");
	const [busy, setBusy] = useState(false);
	const [dropped, setDropped] = useState(false);
	const [recordOpen, setRecordOpen] = useState(false);
	// The panel shows only the media; the import controls open from the button of the header (final design 05).
	const [importOpen, setImportOpen] = useState(false);
	useEffect(() => {
		const open = (): void => setImportOpen(true);
		window.addEventListener(OPEN_IMPORT_EVENT, open);
		return () => window.removeEventListener(OPEN_IMPORT_EVENT, open);
	}, []);
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const pick = canPickFiles(bridge);
	const drops = canDropPaths(bridge);

	const queue = useMemo(
		() => createImportQueue((paths) => runtime.importPaths(paths), setBusy),
		[runtime],
	);

	function submit(): void {
		const paths = parsePastedPaths(text);
		if (paths.length === 0) return;
		setText("");
		queue.add(paths);
	}

	async function choose(): Promise<void> {
		if (bridge?.pickFiles === undefined) return;
		setBusy(true);
		try {
			const picked = cleanPaths(await bridge.pickFiles());
			if (picked.length > 0) await runtime.importPaths(picked);
		} catch {
			// The dialog failed or the shell went away: nothing was picked.
		} finally {
			setBusy(false);
		}
	}

	useEffect(() => {
		if (!canEdit || bridge?.onDropPaths === undefined) return;
		return bridge.onDropPaths((paths) => {
			const clean = cleanPaths(paths);
			if (clean.length > 0) void runtime.importPaths(clean);
		});
	}, [bridge, canEdit, runtime]);

	const visible = entries.filter(
		(entry) => !entry.trashed && !dismissed.includes(entry.asset_id),
	);
	const trashed = entries.filter((entry) => entry.trashed);

	return (
		<section
			aria-label={t("ui.editor.panels.library")}
			className="flex min-h-0 min-w-0 flex-col gap-2 overflow-auto bg-ed-panel p-3"
			onDragOver={(event) => event.preventDefault()}
			onDrop={(event) => {
				event.preventDefault();
				if (!drops) setDropped(true);
			}}
		>
			<PanelTitle>
				{t("ui.editor.panels.library")}
				<Button
					type="button"
					size="sm"
					variant="ghost"
					className="ms-auto h-7 px-2"
					aria-expanded={importOpen}
					aria-controls="library-import"
					data-import-open=""
					onClick={() => setImportOpen((open) => !open)}
				>
					<Plus aria-hidden="true" />
					{t("ui.editor.library.import_open")}
				</Button>
			</PanelTitle>
			<div
				id="library-import"
				hidden={!importOpen}
				className="flex flex-col gap-2"
			>
				<form
					className="flex gap-1"
					onSubmit={(event) => {
						event.preventDefault();
						submit();
					}}
				>
					<Textarea
						value={text}
						rows={1}
						className="min-h-8 resize-y"
						data-import-path=""
						onChange={(event) => setText(event.target.value)}
						onKeyDown={(event) => {
							if (event.key === "Enter" && !event.shiftKey) {
								event.preventDefault();
								submit();
							}
						}}
						placeholder={t("ui.editor.library.path")}
						aria-label={t("ui.editor.library.path")}
						aria-describedby={pick || drops ? undefined : "library-path-why"}
					/>
					<Button
						type="submit"
						size="sm"
						disabled={!canEdit || text.trim() === ""}
					>
						{t("ui.editor.library.import")}
					</Button>
				</form>
				{!(pick || drops) && (
					<p
						id="library-path-why"
						// Read as the description of the field; the text is not repeated in the tree.
						aria-hidden="true"
						className="text-xs text-muted-foreground"
					>
						{t("ui.editor.library.path_why")}
					</p>
				)}
				{pick && (
					<Button
						type="button"
						size="sm"
						variant="outline"
						disabled={busy || !canEdit}
						data-import-choose=""
						onClick={() => void choose()}
					>
						{t("ui.editor.library.choose")}
					</Button>
				)}
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={!canEdit}
					data-record-open=""
					onClick={() => setRecordOpen(true)}
				>
					{t("ui.editor.record.open")}
				</Button>
			</div>
			<RecordDialog
				client={runtime.client}
				projectId={runtime.projectId}
				open={recordOpen}
				onOpenChange={setRecordOpen}
			/>
			{dropped && !drops && (
				<p role="status" className="text-xs text-muted-foreground">
					{t("ui.editor.library.drop")}
				</p>
			)}
			{importError !== null && (
				<div role="alert" className="text-xs text-destructive">
					<p className="font-medium">{errorHeadline(t, importError.code)}</p>
					<p lang="en">
						<UntrustedText>{importError.message}</UntrustedText>
					</p>
					{importError.hint !== undefined && (
						<p lang="en" className="text-muted-foreground">
							<UntrustedText>{importError.hint}</UntrustedText>
						</p>
					)}
				</div>
			)}
			{loading ? (
				<div aria-busy="true" className="flex flex-col gap-2">
					<Skeleton className="h-16" />
					<Skeleton className="h-16" />
				</div>
			) : visible.length === 0 ? (
				<EmptyState
					icon={Film}
					title={t("ui.editor.library.empty")}
					hint={t(
						pick || drops
							? "ui.editor.library.empty_hint_desktop"
							: "ui.editor.library.empty_hint",
					)}
				/>
			) : (
				<ul className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,16rem),1fr))] gap-2">
					{visible.map((entry) => (
						<AssetCard
							key={entry.asset_id}
							entry={entry}
							onDragStart={onAssetDragStart}
						/>
					))}
				</ul>
			)}
			<TrashGroup entries={trashed} />
		</section>
	);
}
