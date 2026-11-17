import { Diamond } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	containsInclusive,
	type FramingKeyframeProperty,
	keyframeAt,
	keyframeRemoveOperations,
	keyframeSetOperations,
	type Operations,
	shownColourKeyframe,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState, useUiState } from "../../state/EditorContext";
import NumberField from "../fields/NumberField";
import { useFieldCommit } from "../useFieldCommit";

// A number of the mask or the crop with the keyframe toggle of a Transform
// row. With keyframes the field shows the one at the playhead (or the nearest
// before it) and an edit becomes a keyframe there; without, the edit is the
// static command the caller builds. The values are the document's: the page
// works out nothing between two keyframes.
export default function KeyframedNumberField({
	itemId,
	property,
	label,
	stored,
	staticOperations,
	step,
	min,
	max,
	suffix,
	percentInput,
	below,
}: {
	itemId: string;
	property: FramingKeyframeProperty;
	label: string;
	// The static value of the property.
	stored: number;
	staticOperations(value: number): Operations;
	step: number;
	min?: number;
	max?: number;
	suffix?: string;
	percentInput?: boolean;
	below?: number;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const playhead = useUiState((s) => s.playheadFrame);
	if (item === undefined) return null;
	const keys = item.keyframes?.[property] ?? [];
	const animated = keys.length > 0;
	const here = keyframeAt(keys, item.start, playhead);
	const inside = containsInclusive(
		{ id: itemId, start: item.start, duration: item.duration },
		playhead,
	);
	const shown = animated
		? (shownColourKeyframe(keys, item.start, playhead)?.value ?? stored)
		: stored;
	return (
		<NumberField
			label={label}
			stored={String(shown)}
			disabled={!canEdit}
			step={step}
			min={min}
			max={max}
			below={below}
			suffix={suffix}
			percentInput={percentInput}
			marker={
				animated
					? t("ui.editor.clip.framing_keyframed", { property: label })
					: undefined
			}
			commit={(value) =>
				send(
					animated
						? keyframeSetOperations({
								itemId,
								property,
								frame: playhead,
								value,
								easing: here?.easing,
							})
						: staticOperations(value),
				)
			}
			trailing={
				<Button
					type="button"
					size="icon-xs"
					variant="ghost"
					disabled={!canEdit || !inside}
					aria-pressed={here !== undefined}
					data-keyframe-button={property}
					aria-label={t(
						here === undefined
							? "ui.editor.clip.color_keyframe_add"
							: "ui.editor.clip.color_keyframe_remove",
						{ property: label },
					)}
					onClick={() =>
						void send(
							here === undefined
								? keyframeSetOperations({
										itemId,
										property,
										frame: playhead,
										value: shown,
									})
								: keyframeRemoveOperations(itemId, property, playhead),
						)
					}
				>
					<Diamond
						aria-hidden="true"
						className={
							here === undefined
								? "size-3"
								: "size-3 fill-current text-ed-handle"
						}
					/>
				</Button>
			}
		/>
	);
}
