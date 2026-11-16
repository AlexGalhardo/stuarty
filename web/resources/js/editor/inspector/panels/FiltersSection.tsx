import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	blurOperations,
	FILTERS,
	type Filter,
	filterOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import { registryRange } from "../fields/registryLimits";
import SelectField from "../fields/SelectField";
import SliderField from "../fields/SliderField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const isFilter = (value: string): value is Filter =>
	(FILTERS as readonly string[]).includes(value);

// The Filters section of a clip (SDD-012 section 12.1, BETA): the colour
// filter with its amount (`clip.set_filter`) and the blur radius
// (`clip.set_blur`), one command per change. Drawn only when the registry
// exposes the commands.
export default function FiltersSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("clip.set_filter", "clip.set_blur");
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (!exposed || item === undefined) return null;
	return (
		<Section title={t("ui.editor.clip.filters_title")}>
			<SelectField
				label={t("ui.editor.clip.filter")}
				stored={item.filter ?? "none"}
				disabled={!canEdit}
				options={FILTERS.map((filter) => ({
					value: filter,
					label: t(`ui.editor.clip.filter_${filter}`),
				}))}
				commit={async (value) =>
					isFilter(value)
						? send(filterOperations(itemId, { filter: value }))
						: { ok: true }
				}
			/>
			<SliderField
				label={t("ui.editor.clip.filter_amount")}
				stored={String(item.filter_amount ?? 1)}
				disabled={!canEdit}
				{...registryRange("clip.set_filter", "amount")}
				step={0.01}
				commit={(amount) => send(filterOperations(itemId, { amount }))}
			/>
			<SliderField
				label={t("ui.editor.clip.blur")}
				stored={String(item.blur ?? 0)}
				disabled={!canEdit}
				{...registryRange("clip.set_blur", "radius")}
				step={0.5}
				commit={(radius) => send(blurOperations(itemId, radius))}
			/>
		</Section>
	);
}
