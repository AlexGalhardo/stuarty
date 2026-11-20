import {
	ChevronDown,
	ChevronRight,
	Eye,
	EyeOff,
	Folder,
	Image as ImageIcon,
	Lock,
	LockOpen,
	PaintBucket,
	Shapes,
	Type,
} from "lucide-react";
import {
	type DragEvent,
	type KeyboardEvent,
	type MouseEvent,
	type ReactNode,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import UntrustedText from "@/components/UntrustedText";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import type { LayerKind } from "../api/types";
import { isFinal, useModelJob } from "../modeledit/state";
import { useImage, useImageStore } from "../state/context";
import {
	type Drop,
	dropMove,
	type Move,
	moveOp,
	type Row,
	step,
	stepIn,
	stepOut,
	treeRows,
} from "./tree";

const ICONS = {
	image: ImageIcon,
	text: Type,
	shape: Shapes,
	fill: PaintBucket,
	group: Folder,
} satisfies Record<LayerKind, unknown>;

// The layer panel (SDD-012 section 24.5): a tree, top layer first. Every action has a key: arrows walk and
// select, `Alt` with the arrows move, `H` and `L` hide and lock, `F2` renames, `Delete` removes, `Ctrl+G` groups.
export default function LayerPanel(): ReactNode {
	const t = useT();
	const store = useImageStore();
	const doc = useImage((s) => s.doc);
	const selection = useImage((s) => s.selection);
	// The layer an operation with a model is working on (SDD-016 section 6.5), while its job has not ended.
	const working = useModelJob((s) =>
		s.tracked !== null && !isFinal(s.tracked.job) ? s.tracked.layerId : null,
	);
	const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
	const [focusId, setFocusId] = useState<string | null>(null);
	const [renaming, setRenaming] = useState<string | null>(null);
	const [dragId, setDragId] = useState<string | null>(null);
	const [dropAt, setDropAt] = useState<{ id: string; where: Drop } | null>(
		null,
	);
	const treeRef = useRef<HTMLDivElement>(null);
	const anchor = useRef<string | null>(null);

	const rows = useMemo(
		() => (doc === null ? [] : treeRows(doc.layers, collapsed)),
		[doc, collapsed],
	);
	const tabStop =
		rows.find((r) => r.layer.id === focusId)?.layer.id ??
		rows.find((r) => selection.includes(r.layer.id))?.layer.id ??
		rows[0]?.layer.id;

	// The selection made on the canvas moves the stop of the tree to it.
	useEffect(() => {
		const first = selection[0];
		if (first !== undefined) setFocusId(first);
	}, [selection]);

	if (doc === null) return null;
	const layers = doc.layers;

	const focusRow = (id: string): void => {
		setFocusId(id);
		treeRef.current
			?.querySelector<HTMLElement>(`[data-layer="${id}"]`)
			?.focus();
	};
	const send = (
		ops: Parameters<ReturnType<typeof store.getState>["send"]>[0],
	) => void store.getState().send(ops);
	const selectOnly = (id: string): void => {
		anchor.current = id;
		store.getState().select([id]);
	};

	const onRowClick = (event: MouseEvent, row: Row): void => {
		const id = row.layer.id;
		setFocusId(id);
		if (event.ctrlKey || event.metaKey) {
			store
				.getState()
				.select(
					selection.includes(id)
						? selection.filter((s) => s !== id)
						: [...selection, id],
				);
			anchor.current = id;
		} else if (event.shiftKey && anchor.current !== null) {
			const from = rows.findIndex((r) => r.layer.id === anchor.current);
			const to = rows.findIndex((r) => r.layer.id === id);
			if (from >= 0 && to >= 0) {
				const [lo, hi] = from < to ? [from, to] : [to, from];
				store.getState().select(rows.slice(lo, hi + 1).map((r) => r.layer.id));
			}
		} else {
			selectOnly(id);
		}
	};

	const toggle = (id: string, next: Set<string> = new Set(collapsed)): void => {
		if (next.has(id)) next.delete(id);
		else next.add(id);
		setCollapsed(next);
	};

	const sendMove = (move: Move | null): void => {
		if (move !== null) send([moveOp(move)]);
	};

	const onKeyDown = (event: KeyboardEvent<HTMLDivElement>, row: Row): void => {
		if (renaming !== null) return;
		const at = rows.findIndex((r) => r.layer.id === row.layer.id);
		const id = row.layer.id;
		const go = (to: number): void => {
			const target = rows[Math.min(Math.max(to, 0), rows.length - 1)];
			if (target === undefined) return;
			focusRow(target.layer.id);
			selectOnly(target.layer.id);
		};

		if (event.altKey && !event.ctrlKey && !event.metaKey) {
			const plan: ((r: Row, all: readonly Row[]) => Move | null) | undefined =
				event.key === "ArrowUp"
					? (r) => step(r, "up")
					: event.key === "ArrowDown"
						? (r) => step(r, "down")
						: event.key === "ArrowLeft"
							? stepOut
							: event.key === "ArrowRight"
								? stepIn
								: undefined;
			if (plan !== undefined) {
				event.preventDefault();
				// Planned when its turn comes, against the document the previous key made.
				send((live) => {
					const all = treeRows(live.layers, new Set());
					const now = all.find((r) => r.layer.id === id);
					const move = now === undefined ? null : plan(now, all);
					return move === null ? [] : [moveOp(move)];
				});
			}
			return;
		}
		if ((event.ctrlKey || event.metaKey) && !event.altKey) {
			const key = event.key.toLowerCase();
			if (key === "g") {
				event.preventDefault();
				if (event.shiftKey) {
					send([{ op: "layer.ungroup", args: { layer_id: id } }]);
				} else {
					const ids = selection.includes(id) ? selection : [id];
					void store
						.getState()
						.send(
							[{ op: "layer.group", args: { layer_ids: ids } }],
							(result) => {
								const made = result.created.slice(0, 1);
								// The row of the new group exists after the next render: put the focus there.
								requestAnimationFrame(() => {
									if (made[0] !== undefined) focusRow(made[0]);
								});
								return made;
							},
						);
				}
			}
			return;
		}
		switch (event.key) {
			case "ArrowDown":
				event.preventDefault();
				go(at + 1);
				break;
			case "ArrowUp":
				event.preventDefault();
				go(at - 1);
				break;
			case "Home":
				event.preventDefault();
				go(0);
				break;
			case "End":
				event.preventDefault();
				go(rows.length - 1);
				break;
			case "ArrowRight":
				event.preventDefault();
				if (row.expanded === false) toggle(id);
				else if (row.expanded === true) go(at + 1);
				break;
			case "ArrowLeft": {
				event.preventDefault();
				if (row.expanded === true) toggle(id);
				else if (row.parentId !== null) {
					const parent = rows.findIndex((r) => r.layer.id === row.parentId);
					go(parent);
				}
				break;
			}
			case "Enter":
			case " ":
				event.preventDefault();
				selectOnly(id);
				break;
			case "F2":
				event.preventDefault();
				setRenaming(id);
				break;
			case "Delete":
			case "Backspace":
				event.preventDefault();
				send(
					(selection.includes(id) ? selection : [id]).map((layer_id) => ({
						op: "layer.remove",
						args: { layer_id },
					})),
				);
				break;
			case "h":
			case "H":
				event.preventDefault();
				send([
					{
						op: "layer.set",
						args: { layer_id: id, visible: !row.layer.visible },
					},
				]);
				break;
			case "l":
			case "L":
				event.preventDefault();
				send([
					{
						op: "layer.set",
						args: { layer_id: id, locked: row.layer.locked !== true },
					},
				]);
				break;
			default:
		}
	};

	const zone = (event: DragEvent<HTMLElement>, row: Row): Drop => {
		const rect = event.currentTarget.getBoundingClientRect();
		const y = (event.clientY - rect.top) / rect.height;
		if (row.layer.kind === "group") {
			if (y < 0.25) return "above";
			if (y > 0.75) return "below";
			return "inside";
		}
		return y < 0.5 ? "above" : "below";
	};

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			{rows.length === 0 ? (
				<div className="p-3 text-sm" data-testid="image-layers-empty">
					<p className="font-medium">{t("ui.image.layers.empty_title")}</p>
					<p className="text-muted-foreground">
						{t("ui.image.layers.empty_body")}
					</p>
				</div>
			) : (
				<div
					ref={treeRef}
					role="tree"
					aria-label={t("ui.image.layers.label")}
					aria-multiselectable="true"
					data-testid="image-layers"
					className="min-h-0 flex-1 overflow-y-auto p-1"
				>
					{rows.map((row) => {
						const { layer } = row;
						const Icon = ICONS[layer.kind];
						const selected = selection.includes(layer.id);
						const state = [
							layer.visible ? null : t("ui.image.layers.hidden"),
							layer.locked ? t("ui.image.layers.locked") : null,
							working === layer.id ? t("ui.image.model.layer_working") : null,
						]
							.filter(Boolean)
							.join(", ");
						const mark = dropAt?.id === layer.id ? dropAt.where : null;
						return (
							<div
								key={layer.id}
								role="treeitem"
								tabIndex={tabStop === layer.id ? 0 : -1}
								aria-level={row.level}
								aria-setsize={row.setSize}
								aria-posinset={row.posInSet}
								aria-selected={selected}
								aria-expanded={row.expanded ?? undefined}
								aria-label={`${layer.name}, ${t(`ui.image.kind.${layer.kind}`)}${state === "" ? "" : `, ${state}`}`}
								data-layer={layer.id}
								data-testid="image-layer-row"
								draggable={renaming !== layer.id}
								onClick={(event) => onRowClick(event, row)}
								onDoubleClick={() => setRenaming(layer.id)}
								onFocus={() => setFocusId(layer.id)}
								onKeyDown={(event) => onKeyDown(event, row)}
								onDragStart={(event) => {
									setDragId(layer.id);
									event.dataTransfer.setData("text/plain", layer.id);
									event.dataTransfer.effectAllowed = "move";
								}}
								onDragEnd={() => {
									setDragId(null);
									setDropAt(null);
								}}
								onDragOver={(event) => {
									if (dragId === null) return;
									const where = zone(event, row);
									if (dropMove(dragId, row, where, layers) === null) {
										setDropAt(null);
										return;
									}
									event.preventDefault();
									setDropAt({ id: layer.id, where });
								}}
								onDrop={(event) => {
									event.preventDefault();
									if (dragId !== null) {
										sendMove(dropMove(dragId, row, zone(event, row), layers));
									}
									setDragId(null);
									setDropAt(null);
								}}
								className={cn(
									"relative flex h-8 cursor-default items-center gap-1 rounded-md pr-1 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring",
									selected
										? "bg-ed-selection/15 text-foreground"
										: "hover:bg-muted",
									mark === "inside" && "ring-2 ring-ed-handle",
									mark === "above" && "border-t-2 border-ed-handle",
									mark === "below" && "border-b-2 border-ed-handle",
									!layer.visible && "opacity-60",
								)}
								style={{ paddingLeft: (row.level - 1) * 16 + 4 }}
							>
								<span
									aria-hidden="true"
									className="flex size-5 shrink-0 items-center justify-center"
									data-testid="image-layer-disclosure"
									onClick={(event) => {
										if (row.expanded === null) return;
										event.stopPropagation();
										toggle(layer.id);
									}}
								>
									{row.expanded === null ? null : row.expanded ? (
										<ChevronDown className="size-4" />
									) : (
										<ChevronRight className="size-4" />
									)}
								</span>
								<Icon
									aria-hidden="true"
									className="size-4 shrink-0 text-muted-foreground"
								/>
								{renaming === layer.id ? (
									<RenameField
										value={layer.name}
										label={t("ui.image.layers.rename")}
										onDone={(name) => {
											setRenaming(null);
											if (name !== null && name !== layer.name) {
												send([
													{
														op: "layer.set",
														args: { layer_id: layer.id, name },
													},
												]);
											}
											focusRow(layer.id);
										}}
									/>
								) : (
									<span className="min-w-0 flex-1 truncate" aria-hidden="true">
										<UntrustedText>{layer.name}</UntrustedText>
									</span>
								)}
								<span
									aria-hidden="true"
									data-testid="image-layer-visible"
									className="flex size-6 shrink-0 items-center justify-center rounded hover:bg-background"
									onClick={(event) => {
										event.stopPropagation();
										send([
											{
												op: "layer.set",
												args: { layer_id: layer.id, visible: !layer.visible },
											},
										]);
									}}
								>
									{layer.visible ? (
										<Eye className="size-4" />
									) : (
										<EyeOff className="size-4" />
									)}
								</span>
								<span
									aria-hidden="true"
									data-testid="image-layer-lock"
									className="flex size-6 shrink-0 items-center justify-center rounded hover:bg-background"
									onClick={(event) => {
										event.stopPropagation();
										send([
											{
												op: "layer.set",
												args: {
													layer_id: layer.id,
													locked: layer.locked !== true,
												},
											},
										]);
									}}
								>
									{layer.locked ? (
										<Lock className="size-4" />
									) : (
										<LockOpen className="size-4 opacity-40" />
									)}
								</span>
							</div>
						);
					})}
				</div>
			)}
		</div>
	);
}

// The name of a layer being edited: Enter keeps it, Escape or leaving the field drops the change.
function RenameField({
	value,
	label,
	onDone,
}: {
	value: string;
	label: string;
	onDone(name: string | null): void;
}): ReactNode {
	const [text, setText] = useState(value);
	const ref = useRef<HTMLInputElement>(null);
	useEffect(() => {
		ref.current?.select();
	}, []);
	return (
		<input
			ref={ref}
			aria-label={label}
			value={text}
			maxLength={120}
			onChange={(event) => setText(event.target.value)}
			onClick={(event) => event.stopPropagation()}
			onKeyDown={(event) => {
				event.stopPropagation();
				if (event.key === "Enter")
					onDone(text.trim() === "" ? null : text.trim());
				if (event.key === "Escape") onDone(null);
			}}
			onBlur={() => onDone(null)}
			className="h-6 min-w-0 flex-1 rounded border bg-background px-1 text-sm"
		/>
	);
}
