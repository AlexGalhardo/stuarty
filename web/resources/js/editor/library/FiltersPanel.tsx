import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import { type Filter, filterOperations } from "../commands/builders";
import { registryRange } from "../commands/registryLimits";
import { useActions, useCanEdit } from "../commands/useActions";
import { useCommandEnum } from "../commands/useExposedCommands";
import { useDocumentState } from "../state/EditorContext";
import ApplyCard from "./ApplyCard";
import PanelFrame from "./PanelFrame";
import { useApplyTarget } from "./useApplyTarget";

const WHY = "filters-why";

// The Filters tab of the material rail (SDD-012 section 6.4): the filters the
// registry lists for `clip.set_filter`, each one command on the selected clips,
// and the amount of the filter of the first one. The core draws nothing here:
// the slider shows the stored amount and sends the one the person chose.
export default function FiltersPanel(): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const names = useCommandEnum("clip.set_filter", "filter");
	const target = useApplyTarget(["clip"]);
	const first = target.ids[0];
	const current = useDocumentState((s) =>
		first === undefined ? undefined : s.document?.items[first]?.filter,
	);
	const stored = useDocumentState((s) =>
		first === undefined ? 1 : (s.document?.items[first]?.filter_amount ?? 1),
	);
	const [draft, setDraft] = useState<number | null>(null);
	const { min, max } = registryRange("clip.set_filter", "amount");
	const active = current !== undefined && current !== "none";
	const send = (changes: { filter?: Filter; amount?: number }): void => {
		void actions.runOperations(
			target.ids.flatMap((itemId) => filterOperations(itemId, changes)),
		);
	};
	const commitAmount = (): void => {
		if (draft === null) return;
		const amount = draft;
		setDraft(null);
		if (amount !== stored) send({ amount });
	};
	return (
		<PanelFrame title={t("ui.editor.panels.filters")}>
			{!target.canApply && (
				<p id={WHY} className="text-xs text-muted-foreground">
					{t("ui.editor.library.select_clip_first")}
				</p>
			)}
			<ul className="grid grid-cols-[repeat(auto-fill,minmax(7rem,1fr))] gap-2">
				{names.map((name) => (
					<li key={name} className="contents">
						<ApplyCard
							name={t(`ui.editor.clip.filter_${name}`)}
							enabled={target.canApply}
							why={WHY}
							pressed={
								target.canApply ? (current ?? "none") === name : undefined
							}
							onApply={() => send({ filter: name as Filter })}
						/>
					</li>
				))}
			</ul>
			<label className="flex items-center gap-2 text-xs">
				{t("ui.editor.clip.filter_amount")}
				<input
					type="range"
					min={min}
					max={max}
					step={0.01}
					value={draft ?? stored}
					disabled={!canEdit || !active}
					aria-describedby={active ? undefined : WHY}
					className="h-8 min-w-0 flex-1 accent-primary"
					onChange={(event) => setDraft(Number(event.target.value))}
					onPointerUp={commitAmount}
					onKeyUp={commitAmount}
					onBlur={commitAmount}
				/>
			</label>
		</PanelFrame>
	);
}
