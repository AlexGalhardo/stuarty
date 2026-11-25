import {
	BringToFront,
	Circle,
	Copy,
	Eraser,
	FileUp,
	Group,
	ImagePlus,
	type LucideIcon,
	Maximize,
	Minus,
	PaintBucket,
	Plus,
	Redo2,
	ScanSearch,
	SendToBack,
	Sparkles,
	Square,
	Stamp,
	Trash2,
	Type,
	Undo2,
	Ungroup,
} from "lucide-react";
import {
	createContext,
	type ReactNode,
	useContext,
	useMemo,
	useState,
} from "react";

import { useT } from "@/lib/i18n";
import type { ModelOperation } from "../api/modelEdit";
import { ZOOM_STEP } from "../canvas/viewport";
import { moveOp, step, treeRows } from "../layers/tree";
import { isFinal, useModelJob } from "../modeledit/state";
import { findLayer, useImage } from "../state/context";
import { useLibrary } from "../state/library";
import { useAddImage } from "./addImage";
import type { ShortcutRow } from "./shortcuts";

export type OperationGroup = "history" | "add" | "arrange" | "model" | "view";

// One thing the editor can do, described once: the tool rail, the "more" menu, the history buttons, the zoom
// bar and the command palette all read this list, so a button and the palette run the very same call.
export interface Operation {
	id: string;
	group: OperationGroup;
	label: string;
	icon: LucideIcon;
	shortcut?: ShortcutRow;
	disabled: boolean;
	testId: string;
	run(): void;
}

interface Dialogs {
	importing: boolean;
	setImporting(open: boolean): void;
	modelOperation: ModelOperation | null;
	setModelOperation(operation: ModelOperation | null): void;
	paletteOpen: boolean;
	setPaletteOpen(open: boolean): void;
}

const DialogsContext = createContext<Dialogs | null>(null);

// The state of the dialogs some operations open (the import of a file, an operation with a model): lifted here
// so a button and the palette open the same one.
export function OperationsProvider({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	const [importing, setImporting] = useState(false);
	const [modelOperation, setModelOperation] = useState<ModelOperation | null>(
		null,
	);
	const [paletteOpen, setPaletteOpen] = useState(false);
	const value = useMemo(
		() => ({
			importing,
			setImporting,
			modelOperation,
			setModelOperation,
			paletteOpen,
			setPaletteOpen,
		}),
		[importing, modelOperation, paletteOpen],
	);
	return (
		<DialogsContext.Provider value={value}>{children}</DialogsContext.Provider>
	);
}

export function useOperationDialogs(): Dialogs {
	const value = useContext(DialogsContext);
	if (value === null)
		throw new Error("useOperationDialogs outside OperationsProvider");
	return value;
}

// The primary tools of the rail, in the order of the final design; every other operation is in "more".
export const PRIMARY_TOOLS = [
	"add-text",
	"add-image",
	"add-rect",
	"add-ellipse",
	"add-fill",
	"model-remove",
	"model-generate",
	"import-open",
] as const;

export function useOperations(): Operation[] {
	const t = useT();
	const store = useImage((s) => s);
	const doc = store.doc;
	const busy = store.busy;
	const selection = store.selection;
	const send = store.send;
	const library = useLibrary();
	const { setPicking } = useAddImage();
	const { setImporting, setModelOperation } = useOperationDialogs();
	const running = useModelJob(
		(s) => s.tracked !== null && !isFinal(s.tracked.job),
	);

	const ready = doc !== null;
	const off = doc === null || busy;
	const none = selection.length === 0 || busy;
	const created = (r: { created: string[] }): string[] => r.created.slice(0, 1);
	// A shape starts a quarter of the canvas wide, centred by the core.
	const side =
		doc === null
			? 200
			: Math.max(Math.round(Math.min(doc.width, doc.height) / 4), 1);
	const logo = library.brand?.logo;
	const only =
		doc !== null && selection.length === 1
			? findLayer(doc.layers, selection[0] ?? "")
			: undefined;
	const row =
		doc !== null && only !== undefined
			? treeRows(doc.layers, new Set()).find((r) => r.layer.id === only.id)
			: undefined;
	const order = (direction: "up" | "down"): void => {
		const move = row === undefined ? null : step(row, direction);
		if (move !== null) void send([moveOp(move)]);
	};
	const removable =
		only !== undefined && only.kind === "image" && only.asset !== undefined;
	const modelOff = off || running;

	return [
		{
			id: "undo",
			group: "history",
			label: t("ui.image.history.undo"),
			icon: Undo2,
			shortcut: "undo",
			disabled: busy || !ready,
			testId: "image-undo",
			run: () => void store.undo(),
		},
		{
			id: "redo",
			group: "history",
			label: t("ui.image.history.redo"),
			icon: Redo2,
			shortcut: "redo",
			disabled: busy || !ready,
			testId: "image-redo",
			run: () => void store.redo(),
		},
		{
			id: "add-text",
			group: "add",
			label: t("ui.image.add.text"),
			icon: Type,
			disabled: off,
			testId: "image-add-text",
			run: () =>
				void send(
					[
						{
							op: "layer.add_text",
							args: { text: t("ui.image.add.text_default") },
						},
					],
					created,
				),
		},
		{
			id: "add-rect",
			group: "add",
			label: t("ui.image.add.rectangle"),
			icon: Square,
			disabled: off,
			testId: "image-add-rect",
			run: () =>
				void send(
					[
						{
							op: "layer.add_shape",
							args: { shape: "rect", w: side, h: side },
						},
					],
					created,
				),
		},
		{
			id: "add-ellipse",
			group: "add",
			label: t("ui.image.add.ellipse"),
			icon: Circle,
			disabled: off,
			testId: "image-add-ellipse",
			run: () =>
				void send(
					[
						{
							op: "layer.add_shape",
							args: { shape: "ellipse", w: side, h: side },
						},
					],
					created,
				),
		},
		{
			id: "add-fill",
			group: "add",
			label: t("ui.image.add.fill"),
			icon: PaintBucket,
			disabled: off,
			testId: "image-add-fill",
			run: () =>
				void send(
					[{ op: "layer.add_fill", args: { color: "#808080" } }],
					created,
				),
		},
		{
			id: "add-image",
			group: "add",
			label: t("ui.image.add.image"),
			icon: ImagePlus,
			disabled: off,
			testId: "image-add-image",
			run: () => setPicking(true),
		},
		{
			id: "import-open",
			group: "add",
			label: t("ui.image.import.open_layer"),
			icon: FileUp,
			disabled: off,
			testId: "image-import-open",
			run: () => setImporting(true),
		},
		{
			id: "place-logo",
			group: "add",
			label: t("ui.image.brand.place_logo"),
			icon: Stamp,
			disabled: off || logo === undefined || logo === "",
			testId: "image-place-logo",
			run: () =>
				void send(
					[
						{
							op: "layer.add_image",
							args: { asset: logo, name: t("ui.image.brand.logo_name") },
						},
					],
					created,
				),
		},
		{
			id: "group",
			group: "arrange",
			label: t("ui.image.arrange.group"),
			icon: Group,
			shortcut: "group",
			disabled: none,
			testId: "image-group",
			run: () =>
				void send(
					[{ op: "layer.group", args: { layer_ids: [...selection] } }],
					(r) => r.created.slice(0, 1),
				),
		},
		{
			id: "ungroup",
			group: "arrange",
			label: t("ui.image.arrange.ungroup"),
			icon: Ungroup,
			shortcut: "ungroup",
			disabled: busy || only?.kind !== "group",
			testId: "image-ungroup",
			run: () =>
				void send(
					[{ op: "layer.ungroup", args: { layer_id: only?.id } }],
					() => [],
				),
		},
		{
			id: "duplicate",
			group: "arrange",
			label: t("ui.image.arrange.duplicate"),
			icon: Copy,
			shortcut: "duplicate",
			disabled: none,
			testId: "image-duplicate",
			run: () =>
				void send(
					selection.map((layer_id) => ({
						op: "layer.duplicate",
						args: { layer_id, dx: 10, dy: 10 },
					})),
					(r) => r.created,
				),
		},
		{
			id: "remove",
			group: "arrange",
			label: t("ui.image.arrange.remove"),
			icon: Trash2,
			shortcut: "remove",
			disabled: none,
			testId: "image-remove",
			run: () =>
				void send(
					selection.map((layer_id) => ({
						op: "layer.remove",
						args: { layer_id },
					})),
				),
		},
		{
			id: "forward",
			group: "arrange",
			label: t("ui.image.arrange.forward"),
			icon: BringToFront,
			shortcut: "forward",
			disabled: busy || row === undefined || step(row, "up") === null,
			testId: "image-forward",
			run: () => order("up"),
		},
		{
			id: "back",
			group: "arrange",
			label: t("ui.image.arrange.back"),
			icon: SendToBack,
			shortcut: "back",
			disabled: busy || row === undefined || step(row, "down") === null,
			testId: "image-back",
			run: () => order("down"),
		},
		{
			id: "model-remove",
			group: "model",
			label: removable
				? t("ui.image.model.remove")
				: t("ui.image.model.remove_needs_layer"),
			icon: Eraser,
			disabled: modelOff || !removable,
			testId: "image-model-remove",
			run: () => setModelOperation("remove_background"),
		},
		{
			id: "model-generate",
			group: "model",
			label: t("ui.image.model.generate"),
			icon: Sparkles,
			disabled: modelOff,
			testId: "image-model-generate",
			run: () => setModelOperation("generate"),
		},
		{
			id: "zoom-out",
			group: "view",
			label: t("ui.image.view.zoom_out"),
			icon: Minus,
			shortcut: "zoomOut",
			disabled: !ready,
			testId: "image-zoom-out",
			run: () => store.zoomBy(1 / ZOOM_STEP),
		},
		{
			id: "zoom-in",
			group: "view",
			label: t("ui.image.view.zoom_in"),
			icon: Plus,
			shortcut: "zoomIn",
			disabled: !ready,
			testId: "image-zoom-in",
			run: () => store.zoomBy(ZOOM_STEP),
		},
		{
			id: "zoom-fit",
			group: "view",
			label: t("ui.image.view.fit"),
			icon: Maximize,
			shortcut: "fit",
			disabled: !ready,
			testId: "image-zoom-fit",
			run: () => store.fitView(),
		},
		{
			id: "zoom-actual",
			group: "view",
			label: t("ui.image.view.actual"),
			icon: ScanSearch,
			shortcut: "actual",
			disabled: !ready,
			testId: "image-zoom-100",
			run: () => store.actualSize(),
		},
	];
}

// One operation by id; the list always holds every id, so a miss is a typo caught by the tests.
export function useOperation(id: string): Operation {
	const found = useOperations().find((o) => o.id === id);
	if (found === undefined) throw new Error(`unknown operation ${id}`);
	return found;
}
