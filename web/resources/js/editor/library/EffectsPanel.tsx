import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { type EffectValue, effectsOperations } from "../commands/builders";
import { useActions } from "../commands/useActions";
import { useCommandEnum } from "../commands/useExposedCommands";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import ApplyCard from "./ApplyCard";
import PanelFrame from "./PanelFrame";
import { useApplyTarget } from "./useApplyTarget";

const WHY = "effects-why";

// The Effects tab of the material rail (SDD-012 section 6.4): the effect kinds
// the registry lists for `clip.set_effects`. A card adds its effect to the
// selected clips, or removes it when they already carry it; the core fills the
// parameters. The card shows the name only: a thumbnail is optional.
export default function EffectsPanel(): ReactNode {
	const t = useT();
	const actions = useActions();
	const runtime = useRuntime();
	const kinds = useCommandEnum("clip.set_effects", "effects", "items", "kind");
	const target = useApplyTarget(["clip"]);
	const first = target.ids[0];
	const carried = useDocumentState((s) =>
		first === undefined
			? ""
			: (s.document?.items[first]?.effects ?? []).map((e) => e.kind).join(" "),
	);
	const toggle = (kind: string): void => {
		const items = runtime.documents.getState().document?.items;
		void actions.runOperations(
			target.ids.flatMap((itemId) => {
				const current: readonly EffectValue[] = items?.[itemId]?.effects ?? [];
				const next = current.some((effect) => effect.kind === kind)
					? current.filter((effect) => effect.kind !== kind)
					: [...current, { kind } as EffectValue];
				return effectsOperations(itemId, next);
			}),
		);
	};
	return (
		<PanelFrame title={t("ui.editor.panels.effects")}>
			{!target.canApply && (
				<p id={WHY} className="text-xs text-muted-foreground">
					{t("ui.editor.library.select_clip_first")}
				</p>
			)}
			<ul className="grid grid-cols-[repeat(auto-fill,minmax(7rem,1fr))] gap-2">
				{kinds.map((kind) => (
					<li key={kind} className="contents">
						<ApplyCard
							name={t(`ui.editor.effects.kind.${kind}`)}
							enabled={target.canApply}
							why={WHY}
							pressed={
								target.canApply ? carried.split(" ").includes(kind) : undefined
							}
							onApply={() => toggle(kind)}
						/>
					</li>
				))}
			</ul>
		</PanelFrame>
	);
}
