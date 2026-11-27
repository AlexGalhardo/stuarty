import { Head } from "@inertiajs/react";
import { Film, Plus, Upload } from "lucide-react";
import {
	type ChangeEvent,
	type DragEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import { EmptyState } from "@/components/EmptyState";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { ListRow } from "@/components/ListRow";
import { LoadingState } from "@/components/LoadingState";
import { PageHeader } from "@/components/PageHeader";
import { SectionTitle } from "@/components/SectionTitle";
import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import {
	canPickFiles,
	DESKTOP_READY_EVENT,
	getDesktopBridge,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";

const PRESETS = [
	{ id: "9:16", dims: "1080×1920" },
	{ id: "16:9", dims: "1920×1080" },
	{ id: "1:1", dims: "1080×1080" },
	{ id: "4:5", dims: "1080×1350" },
] as const;

type PresetId = (typeof PRESETS)[number]["id"];

type Loaded =
	| { state: "loading" }
	| { state: "failed"; failure: Failure }
	| { state: "ready"; edits: ProjectSummary[] };

function formatDuration(seconds: number): string {
	const total = Math.max(0, Math.round(seconds));
	const m = Math.floor(total / 60);
	const s = total % 60;
	return `${m}:${String(s).padStart(2, "0")}`;
}

// Standalone Video Editor launcher (/edit-video, epic 105, SDD-012 section 23.1):
// drop or pick a video file, start from a blank aspect ratio preset, reopen recent
// standalone edits, or auto-open a library asset via ?asset=<hash>.
export default function Index(): ReactNode {
	const t = useT();
	const client = useMemo(() => createClient(new HttpTransport("/api/v1")), []);
	const fileInputRef = useRef<HTMLInputElement | null>(null);
	const [loaded, setLoaded] = useState<Loaded>({ state: "loading" });
	const [busy, setBusy] = useState(false);
	const [busyLabel, setBusyLabel] = useState<string | null>(null);
	const [dragging, setDragging] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [desktopPick, setDesktopPick] = useState(() =>
		canPickFiles(getDesktopBridge()),
	);
	const assetHandled = useRef(false);

	const openEditor = useCallback((projectId: string) => {
		window.location.assign(`/editor/${encodeURIComponent(projectId)}`);
	}, []);

	const openFromPath = useCallback(
		async (filePath: string) => {
			setBusy(true);
			setBusyLabel(t("ui.edit_video.opening_video"));
			setFailure(null);
			try {
				const res = await client.createStandaloneVideo({ path: filePath });
				openEditor(res.project_id);
			} catch (error) {
				setFailure(failureOf(error));
				setBusy(false);
				setBusyLabel(null);
			}
		},
		[client, openEditor, t],
	);

	const openFromFile = useCallback(
		async (file: File) => {
			setBusy(true);
			setBusyLabel(t("ui.edit_video.uploading_video", { name: file.name }));
			setFailure(null);
			try {
				const res = await client.uploadStandaloneVideo(file);
				openEditor(res.project_id);
			} catch (error) {
				setFailure(failureOf(error));
				setBusy(false);
				setBusyLabel(null);
			}
		},
		[client, openEditor, t],
	);

	const openFromPreset = useCallback(
		async (preset: PresetId) => {
			setBusy(true);
			setBusyLabel(t("ui.edit_video.creating_blank"));
			setFailure(null);
			try {
				const res = await client.createStandaloneVideo({ preset });
				openEditor(res.project_id);
			} catch (error) {
				setFailure(failureOf(error));
				setBusy(false);
				setBusyLabel(null);
			}
		},
		[client, openEditor, t],
	);

	// Auto-open when arriving via /edit-video?asset=<hash> (from asset library double-click).
	useEffect(() => {
		if (assetHandled.current || typeof window === "undefined") return;
		const hash = new URLSearchParams(window.location.search).get("asset");
		if (!hash) return;
		assetHandled.current = true;
		setBusy(true);
		setBusyLabel(t("ui.edit_video.opening_asset"));
		setFailure(null);
		client
			.createStandaloneVideo({ asset_hash: hash })
			.then((res) => openEditor(res.project_id))
			.catch((error: unknown) => {
				setFailure(failureOf(error));
				setBusy(false);
				setBusyLabel(null);
			});
	}, [client, openEditor, t]);

	// Load recent standalone video edits.
	useEffect(() => {
		let live = true;
		client
			.listStandaloneVideos()
			.then((res) => {
				if (live) setLoaded({ state: "ready", edits: res.projects });
			})
			.catch((error: unknown) => {
				if (live) setLoaded({ state: "failed", failure: failureOf(error) });
			});
		return () => {
			live = false;
		};
	}, [client]);

	// Desktop bridge integration for native file picker and drag-and-drop paths.
	useEffect(() => {
		let unsub: (() => void) | undefined;
		const attach = (): void => {
			const bridge = getDesktopBridge();
			setDesktopPick(canPickFiles(bridge));
			unsub?.();
			unsub = bridge?.onDropPaths?.((paths) => {
				if (paths.length > 0) void openFromPath(paths[0]);
			});
		};
		attach();
		window.addEventListener(DESKTOP_READY_EVENT, attach);
		return () => {
			window.removeEventListener(DESKTOP_READY_EVENT, attach);
			unsub?.();
		};
	}, [openFromPath]);

	async function handleChooseClick(): Promise<void> {
		if (desktopPick) {
			const bridge = getDesktopBridge();
			const paths = await bridge?.pickFiles?.();
			if (paths && paths.length > 0) {
				await openFromPath(paths[0]);
			}
			return;
		}
		fileInputRef.current?.click();
	}

	function handleFileChange(event: ChangeEvent<HTMLInputElement>): void {
		const file = event.target.files?.[0];
		event.target.value = "";
		if (file) void openFromFile(file);
	}

	function handleDragOver(event: DragEvent<HTMLDivElement>): void {
		event.preventDefault();
		if (!dragging) setDragging(true);
	}

	function handleDragLeave(event: DragEvent<HTMLDivElement>): void {
		event.preventDefault();
		setDragging(false);
	}

	function handleDrop(event: DragEvent<HTMLDivElement>): void {
		event.preventDefault();
		setDragging(false);
		const file = event.dataTransfer.files?.[0];
		if (file) void openFromFile(file);
	}

	return (
		<>
			<Head title={t("ui.edit_video.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-8">
				<PageHeader
					title={t("ui.edit_video.title")}
					description={t("ui.edit_video.intro")}
				/>

				<section
					aria-label={t("ui.edit_video.dropzone_section")}
					className="flex flex-col gap-3"
					onDragOver={handleDragOver}
					onDragLeave={handleDragLeave}
					onDrop={handleDrop}
				>
					<div
						data-testid="edit-video-dropzone"
						className={cn(
							"flex flex-col items-center justify-center gap-4 rounded-xl border-2 border-dashed bg-surface px-6 py-10 text-center transition-colors",
							dragging
								? "border-primary bg-primary/5"
								: "border-border hover:border-primary/50",
						)}
					>
						<div className="flex size-12 items-center justify-center rounded-full bg-surface-sunken text-foreground">
							<Upload aria-hidden="true" className="size-6" />
						</div>
						<div className="flex max-w-md flex-col gap-1">
							<h2 className="text-base font-semibold text-foreground">
								{t("ui.edit_video.drop_title")}
							</h2>
							<p className="text-sm text-muted-foreground">
								{t("ui.edit_video.drop_subtitle")}
							</p>
						</div>
						<input
							ref={fileInputRef}
							type="file"
							accept="video/*"
							tabIndex={-1}
							aria-label={t("ui.edit_video.choose_file")}
							className="sr-only"
							data-testid="edit-video-file-input"
							onChange={handleFileChange}
						/>
						<div className="flex flex-wrap items-center justify-center gap-3">
							<Button
								type="button"
								disabled={busy}
								onClick={() => void handleChooseClick()}
								data-testid="edit-video-choose-file"
							>
								<Film aria-hidden="true" className="size-4" />
								{t("ui.edit_video.choose_file")}
							</Button>
						</div>
						{busy && busyLabel !== null && (
							<p
								role="status"
								className="text-sm font-medium text-muted-foreground"
							>
								{busyLabel}
							</p>
						)}
					</div>
					<FailureNote failure={failure} />
				</section>

				<section
					aria-label={t("ui.edit_video.presets_title")}
					className="flex flex-col gap-3"
					data-testid="edit-video-presets"
				>
					<SectionTitle
						title={t("ui.edit_video.presets_title")}
						description={t("ui.edit_video.presets_intro")}
					/>
					<div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
						{PRESETS.map((preset) => (
							<Button
								key={preset.id}
								type="button"
								variant="outline"
								disabled={busy}
								onClick={() => void openFromPreset(preset.id)}
								data-testid={`edit-video-preset-${preset.id}`}
								className="flex h-auto flex-col items-start gap-1 rounded-lg border-border bg-surface p-4 text-left hover:bg-surface-hover"
							>
								<span className="flex w-full items-center justify-between text-sm font-semibold">
									{t(`ui.edit_video.preset.${preset.id}`)}
									<Plus
										aria-hidden="true"
										className="size-4 text-muted-foreground"
									/>
								</span>
								<span className="text-xs text-muted-foreground">
									{preset.id} · {preset.dims}
								</span>
							</Button>
						))}
					</div>
				</section>

				<section
					aria-label={t("ui.edit_video.recent_title")}
					className="flex flex-col gap-3"
				>
					<SectionTitle
						title={t("ui.edit_video.recent_title")}
						description={t("ui.edit_video.recent_intro")}
					/>
					{loaded.state === "loading" && (
						<LoadingState message={t("ui.edit_video.loading")} />
					)}
					{loaded.state === "failed" && (
						<FailureNote failure={loaded.failure} />
					)}
					{loaded.state === "ready" && loaded.edits.length === 0 && (
						<EmptyState
							icon={Film}
							level={3}
							title={t("ui.edit_video.empty_title")}
							description={t("ui.edit_video.empty")}
						/>
					)}
					{loaded.state === "ready" && loaded.edits.length > 0 && (
						<ul
							data-testid="edit-video-recent"
							className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-surface"
						>
							{loaded.edits.map((item) => (
								<ListRow
									key={item.id}
									leading={<Film aria-hidden="true" className="size-5" />}
									title={
										<a
											href={`/editor/${encodeURIComponent(item.id)}`}
											className="after:absolute after:inset-0 hover:underline"
										>
											<UntrustedText>{item.name}</UntrustedText>
										</a>
									}
									meta={`${item.width}×${item.height} · ${formatDuration(item.duration.seconds)} · r${item.revision}`}
									trailing={
										<Badge variant="secondary">
											{t("ui.edit_video.standalone_badge")}
										</Badge>
									}
									className="relative"
								/>
							))}
						</ul>
					)}
				</section>
			</div>
		</>
	);
}
