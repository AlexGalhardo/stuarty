import {
	BringToFront,
	Copy,
	Group,
	Redo2,
	SendToBack,
	Trash2,
	Undo2,
	Ungroup,
} from "lucide-react";
import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { moveOp, step, treeRows } from "../layers/tree";
import { findLayer, useImage } from "../state/context";
import ToolButton from "./ToolButton";
import { ToolGroup } from "./Toolbar";

// What a button of the toolbar sends for the layers selected: group, ungroup, duplicate, remove and the order of
// the stack. Each is one command of SDD-021 section 3; the server decides whether it is allowed.
export function ArrangeTools(): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const selection = useImage((s) => s.selection);
	const busy = useImage((s) => s.busy);
	const send = useImage((s) => s.send);
	const only =
		doc !== null && selection.length === 1
			? findLayer(doc.layers, selection[0] ?? "")
			: undefined;
	const row =
		doc !== null && only !== undefined
			? treeRows(doc.layers, new Set()).find((r) => r.layer.id === only.id)
			: undefined;
	const none = selection.length === 0 || busy;

	const order = (direction: "up" | "down"): void => {
		const move = row === undefined ? null : step(row, direction);
		if (move !== null) void send([moveOp(move)]);
	};

	return (
		<ToolGroup>
			<ToolButton
				icon={Group}
				label={t("ui.image.arrange.group")}
				shortcut="Ctrl+G"
				disabled={none}
				onClick={() =>
					void send(
						[{ op: "layer.group", args: { layer_ids: [...selection] } }],
						(r) => r.created.slice(0, 1),
					)
				}
				testId="image-group"
			/>
			<ToolButton
				icon={Ungroup}
				label={t("ui.image.arrange.ungroup")}
				shortcut="Ctrl+Shift+G"
				disabled={busy || only?.kind !== "group"}
				onClick={() =>
					void send(
						[{ op: "layer.ungroup", args: { layer_id: only?.id } }],
						() => [],
					)
				}
				testId="image-ungroup"
			/>
			<ToolButton
				icon={Copy}
				label={t("ui.image.arrange.duplicate")}
				shortcut="Ctrl+D"
				disabled={none}
				onClick={() =>
					void send(
						selection.map((layer_id) => ({
							op: "layer.duplicate",
							args: { layer_id, dx: 10, dy: 10 },
						})),
						(r) => r.created,
					)
				}
				testId="image-duplicate"
			/>
			<ToolButton
				icon={Trash2}
				label={t("ui.image.arrange.remove")}
				shortcut="Delete"
				disabled={none}
				onClick={() =>
					void send(
						selection.map((layer_id) => ({
							op: "layer.remove",
							args: { layer_id },
						})),
					)
				}
				testId="image-remove"
			/>
			<ToolButton
				icon={BringToFront}
				label={t("ui.image.arrange.forward")}
				shortcut="]"
				disabled={busy || row === undefined || step(row, "up") === null}
				onClick={() => order("up")}
				testId="image-forward"
			/>
			<ToolButton
				icon={SendToBack}
				label={t("ui.image.arrange.back")}
				shortcut="["
				disabled={busy || row === undefined || step(row, "down") === null}
				onClick={() => order("down")}
				testId="image-back"
			/>
		</ToolGroup>
	);
}

export function HistoryTools(): ReactNode {
	const t = useT();
	const busy = useImage((s) => s.busy);
	const undo = useImage((s) => s.undo);
	const redo = useImage((s) => s.redo);
	const ready = useImage((s) => s.doc !== null);

	return (
		<ToolGroup>
			<ToolButton
				icon={Undo2}
				label={t("ui.image.history.undo")}
				shortcut="Ctrl+Z"
				disabled={busy || !ready}
				onClick={() => void undo()}
				testId="image-undo"
			/>
			<ToolButton
				icon={Redo2}
				label={t("ui.image.history.redo")}
				shortcut="Ctrl+Shift+Z"
				disabled={busy || !ready}
				onClick={() => void redo()}
				testId="image-redo"
			/>
		</ToolGroup>
	);
}
