import { Table2 } from "lucide-react";
import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "@/components/ui/dialog";
import { useT } from "@/lib/i18n";
import {
	displayDuration,
	displayEnd,
	displayStart,
	startsAtOrAfter,
} from "../commands/units";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { readTimeDisplay } from "../state/prefs";
import { selectFps, selectOrderedTrackIds } from "../state/selectors";
import { trackLabel } from "../ui/trackLabel";
import { DEFAULT_FPS, type Fps } from "./geometry";

// The list view of the timeline (SDD-012 section 18.10): the items the canvas draws, as a table a screen
// reader walks by row and column, in track order and then by start. A row's first cell is a button that
// selects the item, the same command as a click on the block. It reads the read model and computes nothing.
export function TimelineRows({
	onChosen,
}: {
	onChosen: () => void;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const fps: Fps = useDocumentState(selectFps) ?? DEFAULT_FPS;
	const order = useDocumentState(selectOrderedTrackIds);
	const tracks = useDocumentState((s) => s.document?.tracks);
	const items = useDocumentState((s) => s.document?.items);
	const assets = useDocumentState((s) => s.document?.assets);
	const selection = useUiState((s) => s.selection.items);
	const mode = readTimeDisplay();
	const kindOf = (id: string): "audio" | "visual" =>
		tracks?.[id]?.kind === "audio" ? "audio" : "visual";

	const rows = order.flatMap((trackId) => {
		const track = tracks?.[trackId];
		if (track === undefined || items === undefined) return [];
		const trackName =
			track.name === undefined || track.name === ""
				? trackLabel(trackId, order, kindOf)
				: track.name;
		return Object.values(items)
			.filter((item) => item.track === trackId)
			.sort((a, b) =>
				startsAtOrAfter(b, a.start)
					? startsAtOrAfter(a, b.start)
						? 0
						: -1
					: 1,
			)
			.map((item) => {
				const typeName = t(`ui.editor.item.${item.type}`);
				const asset =
					item.asset === undefined ? undefined : assets?.[item.asset];
				const label =
					item.name ?? asset?.name ?? item.text?.split("\n")[0] ?? typeName;
				return { item, trackName, typeName, label };
			});
	});

	if (rows.length === 0) {
		return (
			<p className="text-sm text-muted-foreground">
				{t("ui.editor.timeline.table.empty")}
			</p>
		);
	}
	return (
		<div className="max-h-[60dvh] overflow-auto">
			<table className="w-full border-collapse text-sm">
				<caption className="sr-only">
					{t("ui.editor.timeline.table.caption", { count: rows.length })}
				</caption>
				<thead>
					<tr className="text-left">
						{(
							[
								"name",
								"type",
								"track",
								"start",
								"end",
								"duration",
								"state",
							] as const
						).map((column) => (
							<th key={column} scope="col" className="px-2 py-1 font-semibold">
								{t(`ui.editor.timeline.table.${column}`)}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{rows.map(({ item, trackName, typeName, label }) => {
						const selected = selection.includes(item.id);
						const states = [
							selected ? t("ui.editor.timeline.table.selected") : null,
							item.disabled === true
								? t("ui.editor.item.disabled_badge")
								: null,
						].filter((text): text is string => text !== null);
						return (
							<tr key={item.id} data-item-row={item.id} className="border-t">
								<th scope="row" className="px-2 py-1 text-left font-normal">
									<Button
										type="button"
										variant="link"
										size="sm"
										data-testid="timeline-table-select"
										onClick={() => {
											runtime.uiWriter.select([item.id], item.id);
											runtime.uiWriter.focusItem(item.id);
											onChosen();
										}}
									>
										{label}
									</Button>
								</th>
								<td className="px-2 py-1">{typeName}</td>
								<td className="px-2 py-1">{trackName}</td>
								<td className="px-2 py-1 tabular-nums">
									{displayStart(item, fps, mode)}
								</td>
								<td className="px-2 py-1 tabular-nums">
									{displayEnd(item, fps, mode)}
								</td>
								<td className="px-2 py-1 tabular-nums">
									{displayDuration(item, fps, mode)}
								</td>
								<td className="px-2 py-1">{states.join(", ")}</td>
							</tr>
						);
					})}
				</tbody>
			</table>
		</div>
	);
}

export default function TimelineTable(): ReactNode {
	const t = useT();
	const [open, setOpen] = useState(false);
	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					title={t("ui.editor.timeline.table.hint")}
					data-testid="timeline-table-open"
				>
					<Table2 aria-hidden="true" />
					<span className="max-2xl:sr-only">
						{t("ui.editor.timeline.table.open")}
					</span>
				</Button>
			</DialogTrigger>
			<DialogContent className="sm:max-w-4xl">
				<DialogHeader>
					<DialogTitle>{t("ui.editor.timeline.table.title")}</DialogTitle>
					<DialogDescription>
						{t("ui.editor.timeline.table.description")}
					</DialogDescription>
				</DialogHeader>
				{open && <TimelineRows onChosen={() => setOpen(false)} />}
			</DialogContent>
		</Dialog>
	);
}
