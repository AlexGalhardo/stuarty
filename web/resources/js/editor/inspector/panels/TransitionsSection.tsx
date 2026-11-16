import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	type Edge,
	frames,
	TRANSITION_DIRECTIONS,
	TRANSITION_KINDS,
	type TransitionDirection,
	type TransitionKind,
	transitionOperations,
	transitionTakesColor,
	transitionTakesDirection,
} from "../../commands/builders";
import { formatTime, fpsOf } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import { readTimeDisplay } from "../../state/prefs";
import ColorField from "../fields/ColorField";
import SelectField from "../fields/SelectField";
import TimeCommitField from "../fields/TimeCommitField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const isKind = (value: string): value is TransitionKind =>
	(TRANSITION_KINDS as readonly string[]).includes(value);

const isDirection = (value: string): value is TransitionDirection =>
	(TRANSITION_DIRECTIONS as readonly string[]).includes(value);

const EDGES: readonly Edge[] = ["in", "out"];

// The Transitions section of SDD-012 section 12.1: for the in and the out edge
// the kind and the length, each one `item.set_transition`. Kind none sends
// `kind: null`. The length of a transition that exists is copied from the read
// model when only the kind changes; the core judges whether it fits. A dip also
// has its colour and a whip or a wipe its direction, which go with that kind
// only, so a change of kind starts from the defaults and a change of length keeps
// them.
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
						{current !== undefined && transitionTakesColor(current.kind) && (
							<ColorField
								label={t(`ui.editor.transitions.${edge}_color`)}
								stored={current.color ?? "#000000"}
								disabled={!canEdit}
								commit={(color) =>
									send(
										transitionOperations({
											itemId,
											edge,
											kind: current.kind,
											duration: frames(current.duration),
											color,
										}),
									)
								}
							/>
						)}
						{current !== undefined &&
							transitionTakesDirection(current.kind) && (
								<SelectField
									label={t(`ui.editor.transitions.${edge}_direction`)}
									stored={current.direction ?? "left"}
									disabled={!canEdit}
									options={TRANSITION_DIRECTIONS.map((direction) => ({
										value: direction,
										label: t(`ui.editor.transitions.direction.${direction}`),
									}))}
									commit={async (direction) =>
										isDirection(direction)
											? send(
													transitionOperations({
														itemId,
														edge,
														kind: current.kind,
														duration: frames(current.duration),
														direction,
													}),
												)
											: { ok: true }
									}
								/>
							)}
						<TimeCommitField
							label={t(`ui.editor.transitions.${edge}_duration`)}
							stored={
								current === undefined
									? ""
									: formatTime(current.duration, fps, readTimeDisplay())
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
												color: current.color,
												direction: current.direction,
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
