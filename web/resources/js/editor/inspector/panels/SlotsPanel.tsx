import { type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { LintFinding } from "../../api/types";
import { type SlotFill, slotFillOperations } from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import Section from "../Section";

interface Row {
	itemId: string;
	name: string;
	kind: "media" | "text" | "caption";
	required: boolean;
	filled: boolean;
}

function SlotRow({ row }: { row: Row }): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const assets = useDocumentState((s) => s.document?.assets);
	const [draft, setDraft] = useState("");
	const id = `slot-${row.itemId}`;

	const fill = (value: SlotFill): void => {
		void actions.runOperations(slotFillOperations(row.name, value));
	};

	return (
		<li
			className="flex flex-col gap-1 rounded-md border p-2"
			data-slot-row={row.name}
		>
			<p className="flex items-center gap-2 text-sm">
				<span className="font-medium">
					<UntrustedText>{row.name}</UntrustedText>
				</span>
				<span className="text-xs text-muted-foreground">
					{t(`ui.editor.slots.kinds.${row.kind}`)}
					{row.required ? ` · ${t("ui.editor.slots.required")}` : ""}
				</span>
			</p>
			{row.filled ? (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.slots.filled")}
				</p>
			) : row.kind === "caption" ? (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.slots.caption_hint")}
				</p>
			) : (
				<div className="flex items-end gap-2">
					<div className="flex min-w-0 flex-1 flex-col gap-1">
						<Label htmlFor={id} className="text-xs">
							{row.kind === "media"
								? t("ui.editor.slots.asset")
								: t("ui.editor.slots.text")}
						</Label>
						{row.kind === "media" ? (
							<select
								id={id}
								value={draft}
								disabled={!canEdit}
								className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
								onChange={(event) => setDraft(event.target.value)}
							>
								<option value="">{t("ui.editor.slots.choose")}</option>
								{Object.values(assets ?? {}).map((asset) => (
									<option key={asset.id} value={asset.id}>
										{asset.name}
									</option>
								))}
							</select>
						) : (
							<Input
								id={id}
								value={draft}
								maxLength={2000}
								disabled={!canEdit}
								className="h-8"
								onChange={(event) => setDraft(event.target.value)}
							/>
						)}
					</div>
					<Button
						type="button"
						size="sm"
						disabled={!canEdit || draft === ""}
						onClick={() =>
							fill(row.kind === "media" ? { asset_id: draft } : { text: draft })
						}
					>
						{t("ui.editor.slots.fill")}
					</Button>
				</div>
			)}
		</li>
	);
}

// The "Template slots" panel of SDD-012 sections 9.1 and 12.1: one row per slot
// of the document with an asset picker or a text field and "Fill" (`slot.fill`).
// The required slots still open come from the lint of the core
// (`L_SLOT_UNFILLED`), read again whenever the revision changes; the panel
// decides nothing itself.
export default function SlotsPanel(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const revision = useDocumentState((s) => s.revision);
	const items = useDocumentState((s) => s.document?.items);
	const [open, setOpen] = useState<readonly LintFinding[]>([]);

	// biome-ignore lint/correctness/useExhaustiveDependencies: a new revision is the reason to read the lint again.
	useEffect(() => {
		let live = true;
		runtime.client
			.getLint(runtime.projectId)
			.then((result) => {
				if (live)
					setOpen(result.findings.filter((f) => f.code === "L_SLOT_UNFILLED"));
			})
			.catch(() => undefined);
		return () => {
			live = false;
		};
	}, [runtime, revision]);

	const rows: Row[] = Object.values(items ?? {}).flatMap((item) =>
		item.slot === undefined
			? []
			: [
					{
						itemId: item.id,
						name: item.slot.name,
						kind: item.slot.kind,
						required: item.slot.required ?? true,
						filled: item.slot.filled ?? false,
					},
				],
	);
	if (rows.length === 0) return null;

	return (
		<Section title={t("ui.editor.slots.title")}>
			{open.length > 0 && (
				<div
					role="status"
					data-unfilled=""
					className="text-xs text-destructive"
				>
					<p className="font-medium">
						{t("ui.editor.slots.unfilled", { count: open.length })}
					</p>
					<ul>
						{open.map((finding) => (
							<li key={`${finding.item ?? ""}:${finding.message}`} lang="en">
								<UntrustedText>{finding.message}</UntrustedText>
							</li>
						))}
					</ul>
				</div>
			)}
			<ul className="flex flex-col gap-2">
				{rows.map((row) => (
					<SlotRow key={row.itemId} row={row} />
				))}
			</ul>
		</Section>
	);
}
