import { Head, Link } from "@inertiajs/react";
import { Image as ImageIcon, Upload } from "lucide-react";
import {
	type ChangeEvent,
	type DragEvent,
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
import { FieldLabel } from "@/components/FieldHelp";
import { ListRow } from "@/components/ListRow";
import { LoadingState } from "@/components/LoadingState";
import { PageHeader } from "@/components/PageHeader";
import { SectionTitle } from "@/components/SectionTitle";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/cn";
import {
	canPickFiles,
	DESKTOP_READY_EVENT,
	getDesktopBridge,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import { HttpTransport } from "../../editor/api/http";
import { API_BASE, createImageApi } from "../../image-editor/api/imageApi";
import type { ImageSummary } from "../../image-editor/api/types";
import ImportDialog, {
	type ImportValues,
} from "../../image-editor/import/ImportDialog";
import NewFromTemplate from "../../image-editor/templates/NewFromTemplate";

// The sizes a new image starts from: the names of the presets of SDD-021 section 6, and a size of your own.
const PRESETS = ["video_thumbnail", "square_post", "vertical_cover"] as const;
const CUSTOM = "custom";
const NAME_MAX = 120;

type Loaded =
	| { state: "loading" }
	| { state: "failed"; failure: Failure }
	| { state: "ready"; images: ImageSummary[] };

// The documents or the templates as rows of the shared list (a whole row is the link), each with its size and
// the number of layers the server reports.
function ImageRows({
	images,
	testId,
}: {
	images: readonly ImageSummary[];
	testId: string;
}): ReactNode {
	const t = useT();
	return (
		<ul
			data-testid={testId}
			className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-surface"
		>
			{images.map((image) => (
				<ListRow
					key={image.image_id}
					leading={<ImageIcon aria-hidden="true" className="size-5" />}
					title={
						<Link
							href={`/edit-image/${image.image_id}`}
							className="after:absolute after:inset-0 hover:underline"
						>
							<UntrustedText>{image.name}</UntrustedText>
						</Link>
					}
					meta={`${t("ui.image.editor.size", { w: image.width, h: image.height })} · ${t("ui.image.index.layers", { count: image.layers })}`}
					className="relative"
				/>
			))}
		</ul>
	);
}

// Edit Image (RF-341, SDD-012 section 24.1): the documents, a new blank image and a new one from a template.
export default function Index(): ReactNode {
	const t = useT();
	const api = useMemo(() => createImageApi(new HttpTransport(API_BASE)), []);
	const [loaded, setLoaded] = useState<Loaded>({ state: "loading" });
	const [name, setName] = useState("");
	const [preset, setPreset] = useState<string>(PRESETS[0]);
	const [width, setWidth] = useState("1280");
	const [height, setHeight] = useState("720");
	// `image.create` with `template`: a template is made here and filled from "New from template".
	const [asTemplate, setAsTemplate] = useState(false);
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	// "Import image": a file of the computer becomes a new document (SDD-021 section 10).
	const [importing, setImporting] = useState(false);
	const [importBusy, setImportBusy] = useState(false);
	const [importFailure, setImportFailure] = useState<Failure | null>(null);
	// Standalone instant dropzone and ?asset=<hash> quick-open (epic 105, SDD-012 section 23.1).
	const quickFileRef = useRef<HTMLInputElement | null>(null);
	const [quickBusy, setQuickBusy] = useState(false);
	const [quickLabel, setQuickLabel] = useState<string | null>(null);
	const [quickFailure, setQuickFailure] = useState<Failure | null>(null);
	const [dragging, setDragging] = useState(false);
	const [desktopPick, setDesktopPick] = useState(() =>
		canPickFiles(getDesktopBridge()),
	);
	const assetHandled = useRef(false);

	const openQuickPath = useCallback(
		async (filePath: string) => {
			setQuickBusy(true);
			setQuickLabel(t("ui.image.dropzone.opening"));
			setQuickFailure(null);
			try {
				const result = await api.importImage({ path: filePath });
				if (result.state === "pending" || result.image_id === undefined) {
					setQuickFailure({ code: "E_ASSET_NOT_READY", message: result.text });
					setQuickBusy(false);
					setQuickLabel(null);
					return;
				}
				window.location.assign(`/edit-image/${result.image_id}`);
			} catch (error) {
				setQuickFailure(failureOf(error));
				setQuickBusy(false);
				setQuickLabel(null);
			}
		},
		[api, t],
	);

	const openQuickFile = useCallback(
		async (file: File) => {
			setQuickBusy(true);
			setQuickLabel(t("ui.image.dropzone.uploading", { name: file.name }));
			setQuickFailure(null);
			try {
				const result = await api.importFile(file, {});
				if (result.state === "pending" || result.image_id === undefined) {
					setQuickFailure({ code: "E_ASSET_NOT_READY", message: result.text });
					setQuickBusy(false);
					setQuickLabel(null);
					return;
				}
				window.location.assign(`/edit-image/${result.image_id}`);
			} catch (error) {
				setQuickFailure(failureOf(error));
				setQuickBusy(false);
				setQuickLabel(null);
			}
		},
		[api, t],
	);

	// Auto-open when arriving via /edit-image?asset=<hash>.
	useEffect(() => {
		if (assetHandled.current || typeof window === "undefined") return;
		const hash = new URLSearchParams(window.location.search).get("asset");
		if (!hash) return;
		assetHandled.current = true;
		setQuickBusy(true);
		setQuickLabel(t("ui.image.dropzone.opening_asset"));
		setQuickFailure(null);
		api
			.openFromAsset(hash)
			.then((result) => {
				if (result.state === "pending" || result.image_id === undefined) {
					setQuickFailure({ code: "E_ASSET_NOT_READY", message: result.text });
					setQuickBusy(false);
					setQuickLabel(null);
					return;
				}
				window.location.assign(`/edit-image/${result.image_id}`);
			})
			.catch((error: unknown) => {
				setQuickFailure(failureOf(error));
				setQuickBusy(false);
				setQuickLabel(null);
			});
	}, [api, t]);

	// Desktop bridge integration for native file picker and drag-and-drop paths.
	useEffect(() => {
		let unsub: (() => void) | undefined;
		const attach = (): void => {
			const bridge = getDesktopBridge();
			setDesktopPick(canPickFiles(bridge));
			unsub?.();
			unsub = bridge?.onDropPaths?.((paths) => {
				if (paths.length > 0) void openQuickPath(paths[0]);
			});
		};
		attach();
		window.addEventListener(DESKTOP_READY_EVENT, attach);
		return () => {
			window.removeEventListener(DESKTOP_READY_EVENT, attach);
			unsub?.();
		};
	}, [openQuickPath]);

	async function handleQuickChoose(): Promise<void> {
		if (desktopPick) {
			const bridge = getDesktopBridge();
			const paths = await bridge?.pickFiles?.();
			if (paths && paths.length > 0) {
				await openQuickPath(paths[0]);
			}
			return;
		}
		quickFileRef.current?.click();
	}

	function handleQuickFileChange(event: ChangeEvent<HTMLInputElement>): void {
		const file = event.target.files?.[0];
		event.target.value = "";
		if (file) void openQuickFile(file);
	}

	function handleDragOver(event: DragEvent<HTMLElement>): void {
		event.preventDefault();
		if (!dragging) setDragging(true);
	}

	function handleDragLeave(event: DragEvent<HTMLElement>): void {
		event.preventDefault();
		setDragging(false);
	}

	function handleDrop(event: DragEvent<HTMLElement>): void {
		event.preventDefault();
		setDragging(false);
		const file = event.dataTransfer.files?.[0];
		if (file) void openQuickFile(file);
	}

	useEffect(() => {
		let live = true;
		api
			.list()
			.then((result) => {
				if (live) setLoaded({ state: "ready", images: result.images });
			})
			.catch((error: unknown) => {
				if (live) setLoaded({ state: "failed", failure: failureOf(error) });
			});
		return () => {
			live = false;
		};
	}, [api]);

	async function importImage(values: ImportValues): Promise<void> {
		setImportBusy(true);
		setImportFailure(null);
		try {
			const options = {
				...(values.name === undefined ? {} : { name: values.name }),
				...(values.preset === undefined ? {} : { preset: values.preset }),
			};
			const result =
				values.file === undefined
					? await api.importImage({ ...options, path: values.path })
					: await api.importFile(values.file, options);
			if (result.state === "pending" || result.image_id === undefined) {
				// The file is still being read by the server: nothing was made, and the person may try again.
				setImportFailure({ code: "E_ASSET_NOT_READY", message: result.text });
				setImportBusy(false);
				return;
			}
			window.location.assign(`/edit-image/${result.image_id}`);
		} catch (error) {
			setImportFailure(failureOf(error));
			setImportBusy(false);
		}
	}

	async function create(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			const made = await api.create({
				...(name.trim() === "" ? {} : { name: name.trim() }),
				...(preset === CUSTOM
					? { width: Number(width), height: Number(height) }
					: { preset }),
				...(asTemplate ? { template: true } : {}),
			});
			window.location.assign(`/edit-image/${made.image_id}`);
		} catch (error) {
			setFailure(failureOf(error));
			setBusy(false);
		}
	}

	const documents =
		loaded.state === "ready" ? loaded.images.filter((i) => !i.template) : [];
	// The templates have a list of their own, so a person can open one and give it its slots.
	const templates =
		loaded.state === "ready" ? loaded.images.filter((i) => i.template) : [];

	return (
		<>
			<Head title={t("ui.image.index.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-8">
				<PageHeader
					title={t("ui.image.index.title")}
					description={t("ui.image.index.intro")}
				/>

				<section
					aria-label={t("ui.image.dropzone.section")}
					className="flex flex-col gap-3"
					onDragOver={handleDragOver}
					onDragLeave={handleDragLeave}
					onDrop={handleDrop}
				>
					<div
						data-testid="edit-image-dropzone"
						className={cn(
							"flex flex-col items-center justify-center gap-4 rounded-xl border-2 border-dashed bg-surface px-6 py-8 text-center transition-colors",
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
								{t("ui.image.dropzone.title")}
							</h2>
							<p className="text-sm text-muted-foreground">
								{t("ui.image.dropzone.subtitle")}
							</p>
						</div>
						<input
							ref={quickFileRef}
							type="file"
							accept="image/*"
							className="sr-only"
							data-testid="edit-image-file-input"
							onChange={handleQuickFileChange}
						/>
						<div className="flex flex-wrap items-center justify-center gap-3">
							<Button
								type="button"
								disabled={quickBusy}
								onClick={() => void handleQuickChoose()}
								data-testid="edit-image-choose-file"
							>
								<ImageIcon aria-hidden="true" className="size-4" />
								{t("ui.image.dropzone.choose")}
							</Button>
						</div>
						{quickBusy && quickLabel !== null && (
							<p
								role="status"
								className="text-sm font-medium text-muted-foreground"
							>
								{quickLabel}
							</p>
						)}
					</div>
					<FailureNote failure={quickFailure} />
				</section>

				<section
					aria-label={t("ui.image.index.new")}
					className="flex flex-col gap-3"
				>
					<SectionTitle title={t("ui.image.index.new")} />
					<form
						onSubmit={(event) => void create(event)}
						className="flex flex-wrap items-end gap-3 rounded-lg border border-border bg-surface p-4"
						data-testid="image-new-form"
					>
						<div className="flex flex-col gap-1">
							<FieldLabel
								htmlFor="image-new-name"
								label={t("ui.image.index.name")}
								help={t("ui.help.g5.img_new_name_help")}
							/>
							<Input
								id="image-new-name"
								value={name}
								maxLength={NAME_MAX}
								onChange={(event) => setName(event.target.value)}
								placeholder={t("ui.image.index.name_placeholder")}
							/>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="image-new-size">{t("ui.image.index.size")}</Label>
							<select
								id="image-new-size"
								value={preset}
								onChange={(event) => setPreset(event.target.value)}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								{PRESETS.map((p) => (
									<option key={p} value={p}>
										{t(`ui.image.preset.${p}`)}
									</option>
								))}
								<option value={CUSTOM}>{t("ui.image.preset.custom")}</option>
							</select>
						</div>
						{preset === CUSTOM && (
							<>
								<div className="flex w-28 flex-col gap-1">
									<FieldLabel
										htmlFor="image-new-width"
										label={t("ui.image.index.width")}
										help={t("ui.help.g5.img_dim_w_help")}
									/>
									<Input
										id="image-new-width"
										inputMode="numeric"
										placeholder={t("ui.help.g5.img_dim_w_ph")}
										value={width}
										onChange={(event) => setWidth(event.target.value)}
									/>
								</div>
								<div className="flex w-28 flex-col gap-1">
									<FieldLabel
										htmlFor="image-new-height"
										label={t("ui.image.index.height")}
										help={t("ui.help.g5.img_dim_h_help")}
									/>
									<Input
										id="image-new-height"
										inputMode="numeric"
										placeholder={t("ui.help.g5.img_dim_h_ph")}
										value={height}
										onChange={(event) => setHeight(event.target.value)}
									/>
								</div>
							</>
						)}
						<label className="flex h-8 items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={asTemplate}
								onChange={(event) => setAsTemplate(event.target.checked)}
								data-testid="image-new-template"
							/>
							{t("ui.image.index.as_template")}
						</label>
						<Button type="submit" disabled={busy}>
							{t("ui.image.index.create")}
						</Button>
						<Button
							type="button"
							variant="outline"
							onClick={() => {
								setImportFailure(null);
								setImporting(true);
							}}
							data-testid="image-import-open"
						>
							{t("ui.image.import.open_new")}
						</Button>
					</form>
					<FailureNote failure={failure} />
					<ImportDialog
						open={importing}
						onOpenChange={setImporting}
						mode="new"
						busy={importBusy}
						failure={importFailure}
						onImport={(values) => void importImage(values)}
					/>
				</section>

				<NewFromTemplate api={api} />

				<section
					aria-label={t("ui.image.index.documents")}
					className="flex flex-col gap-3"
				>
					<SectionTitle title={t("ui.image.index.documents")} />
					{loaded.state === "loading" && (
						<LoadingState message={t("ui.image.index.loading")} />
					)}
					{loaded.state === "failed" && (
						<FailureNote failure={loaded.failure} />
					)}
					{loaded.state === "ready" && documents.length === 0 && (
						<EmptyState
							icon={ImageIcon}
							level={3}
							title={t("ui.image.index.empty_title")}
							description={t("ui.image.index.empty")}
						/>
					)}
					{documents.length > 0 && (
						<ImageRows images={documents} testId="image-list" />
					)}
				</section>

				{templates.length > 0 && (
					<section
						aria-label={t("ui.image.index.templates")}
						className="flex flex-col gap-3"
					>
						<SectionTitle title={t("ui.image.index.templates")} />
						<ImageRows images={templates} testId="image-template-list" />
					</section>
				)}
			</div>
		</>
	);
}
