import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	type Edge,
	frames,
	TRANSITION_KINDS,
	type TransitionKind,
	transitionOperations,
} from "../../commands/builders";
import { formatTime, fpsOf } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import SelectField from "../fields/SelectField";
import TimeCommitField from "../fields/TimeCommitField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const isKind = (value: string): value is TransitionKind =>
	(TRANSITION_KINDS as readonly string[]).includes(value);

const EDGES: readonly Edge[] = ["in", "out"];

// The Transitions section of SDD-012 section 12.1: for the in and the out edge
// the kind and the length, each one `item.set_transition`. Kind none sends
// `kind: null`. The length of a transition that exists is copied from the read
// model when only the kind changes; the core judges whether it fits.
export default function TransitionsSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const rate = useDocumentState((s) => s.document?.settings.fps);
	if (item === undefined) return null;
	const fps = fpsOf(rate);

	return (
		<Section title={t("ui.editor.transitions.title")}>
			{EDGES.map((edge) => {
				const current =
					edge === "in" ? item.transition_in : item.transition_out;
				return (
					<div key={edge} data-transition-edge={edge} className="contents">
						<SelectField
							label={t(`ui.editor.transitions.${edge}`)}
							stored={current?.kind ?? ""}
							disabled={!canEdit}
							options={[
								{ value: "", label: t("ui.editor.transitions.none") },
								...TRANSITION_KINDS.map((kind) => ({
									value: kind,
									label: t(`ui.editor.transitions.kind.${kind}`),
								})),
							]}
							commit={(value) =>
								send(
									transitionOperations({
										itemId,
										edge,
										kind: isKind(value) ? value : null,
										duration:
											isKind(value) && current !== undefined
												? frames(current.duration)
												: undefined,
									}),
								)
							}
						/>
						<TimeCommitField
							label={t(`ui.editor.transitions.${edge}_duration`)}
							stored={
								current === undefined
									? ""
									: formatTime(current.duration, fps, "timecode")
							}
							disabled={!canEdit || current === undefined}
							commit={(duration) =>
								current === undefined
									? Promise.resolve({ ok: true })
									: send(
											transitionOperations({
												itemId,
												edge,
												kind: current.kind,
												duration,
											}),
										)
							}
						/>
					</div>
				);
			})}
		</Section>
	);
}
