import {
	type ReactNode,
	useEffect,
	useState,
	useSyncExternalStore,
} from "react";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useT } from "@/lib/i18n";
import { useCanEdit } from "../commands/useActions";
import { useStore } from "../state/createStore";
import { useRuntime } from "../state/EditorContext";
import { errorHeadline } from "../ui/errorHeadline";
import AssetCard, { type AssetDragStart } from "./AssetCard";
import {
	canDropPaths,
	canPickFiles,
	cleanPaths,
	getDesktopBridge,
	subscribeDesktopBridge,
} from "./desktopBridge";
import { parsePastedPaths } from "./importPaths";
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
	const entries = useStore(runtime.library, (s) => s.entries);
	const dismissed = useStore(runtime.library, (s) => s.dismissed);
	const importError = useStore(runtime.library, (s) => s.importError);
	const [text, setText] = useState("");
	const [busy, setBusy] = useState(false);
	const [dropped, setDropped] = useState(false);
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const pick = canPickFiles(bridge);
	const drops = canDropPaths(bridge);

	async function submit(): Promise<void> {
		const paths = parsePastedPaths(text);
		if (paths.length === 0) return;
		setBusy(true);
		try {
			await runtime.importPaths(paths);
			setText("");
		} finally {
			setBusy(false);
		}
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
			<h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
				{t("ui.editor.panels.library")}
			</h2>
			<form
				className="flex gap-1"
				onSubmit={(event) => {
					event.preventDefault();
					void submit();
				}}
			>
				<Input
					value={text}
					onChange={(event) => setText(event.target.value)}
					placeholder={t("ui.editor.library.path")}
					aria-label={t("ui.editor.library.path")}
				/>
				<Button
					type="submit"
					size="sm"
					disabled={busy || !canEdit || text.trim() === ""}
				>
					{t("ui.editor.library.import")}
				</Button>
			</form>
			{pick && (
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={busy || !canEdit}
					onClick={() => void choose()}
				>
					{t("ui.editor.library.choose")}
				</Button>
			)}
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
			{visible.length === 0 ? (
				<p className="text-sm text-muted-foreground">
					{t(
						pick || drops
							? "ui.editor.library.empty_desktop"
							: "ui.editor.library.empty",
					)}
				</p>
			) : (
				<ul className="flex flex-col gap-2">
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
