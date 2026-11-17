import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import {
	type Edge,
	type TransitionKind,
	transitionOperations,
} from "../commands/builders";
import { useActions } from "../commands/useActions";
import { useCommandEnum } from "../commands/useExposedCommands";
import ApplyCard from "./ApplyCard";
import PanelFrame from "./PanelFrame";
import { useApplyTarget } from "./useApplyTarget";

const WHY = "transitions-why";

// The Transitions tab of the material rail (SDD-012 section 6.4): the kinds the
// registry lists for `item.set_transition`, applied to the selection at the
// chosen edge with the length the core defaults to.
export default function TransitionsPanel(): ReactNode {
	const t = useT();
	const actions = useActions();
	const kinds = useCommandEnum("item.set_transition", "kind");
	const target = useApplyTarget(["clip", "text", "caption", "shape"]);
	const [edge, setEdge] = useState<Edge>("in");
	return (
		<PanelFrame title={t("ui.editor.panels.transitions")}>
			<fieldset className="flex gap-3 text-sm">
				<legend className="sr-only">{t("ui.editor.animation.edge")}</legend>
				{(["in", "out"] as const).map((value) => (
					<label key={value} className="flex items-center gap-1">
						<input
							type="radio"
							name="library-transition-edge"
							checked={edge === value}
							onChange={() => setEdge(value)}
						/>
						{t(`ui.editor.animation.edge_${value}`)}
					</label>
				))}
			</fieldset>
			{!target.canApply && (
				<p id={WHY} className="text-xs text-muted-foreground">
					{t("ui.editor.library.select_clip_first")}
				</p>
			)}
			<ul className="grid grid-cols-[repeat(auto-fill,minmax(7rem,1fr))] gap-2">
				{kinds.map((kind) => (
					<li key={kind} className="contents">
						<ApplyCard
							name={t(`ui.editor.transitions.kind.${kind}`)}
							enabled={target.canApply}
							why={WHY}
							onApply={() =>
								void actions.runOperations(
									target.ids.flatMap((itemId) =>
										transitionOperations({
											itemId,
											edge,
											kind: kind as TransitionKind,
										}),
									),
								)
							}
						/>
					</li>
				))}
			</ul>
		</PanelFrame>
	);
}
