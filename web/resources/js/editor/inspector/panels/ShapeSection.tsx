import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	SHAPES,
	type ShapeKind,
	shapeSetOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import ColorField from "../fields/ColorField";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const isShape = (value: string): value is ShapeKind =>
	(SHAPES as readonly string[]).includes(value);

// The Shape section of SDD-012 section 12.1: the kind, the size, the corner
// radius (only for a rounded rectangle), the fill and the stroke. Each field
// is one `shape.set` with the changed key only; the core judges the rest.
export default function ShapeSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (item === undefined) return null;
	const shape = item.shape ?? "rect";
	const number = (
		key: "width" | "height" | "radius" | "stroke_width",
		label: string,
		min: number,
		stored: number,
	): ReactNode => (
		<NumberField
			label={label}
			stored={String(stored)}
			disabled={!canEdit}
			min={min}
			step={1}
			commit={(value) => send(shapeSetOperations(itemId, { [key]: value }))}
		/>
	);
	const colour = (
		key: "fill" | "stroke_color",
		label: string,
		stored: string,
	): ReactNode => (
		<ColorField
			label={label}
			stored={stored}
			disabled={!canEdit}
			commit={(hex) => send(shapeSetOperations(itemId, { [key]: hex }))}
		/>
	);
	return (
		<Section title={t("ui.editor.shape.title")}>
			<SelectField
				label={t("ui.editor.shape.kind")}
				stored={shape}
				disabled={!canEdit}
				options={SHAPES.map((kind) => ({
					value: kind,
					label: t(`ui.editor.shape.${kind}`),
				}))}
				commit={async (value) =>
					isShape(value)
						? send(shapeSetOperations(itemId, { shape: value }))
						: { ok: true }
				}
			/>
			{number("width", t("ui.editor.shape.width"), 1, item.width ?? 0)}
			{number("height", t("ui.editor.shape.height"), 1, item.height ?? 0)}
			{shape === "rounded_rect" &&
				number("radius", t("ui.editor.shape.radius"), 0, item.radius ?? 0)}
			{colour("fill", t("ui.editor.shape.fill"), item.fill ?? "#FFFFFF")}
			{colour(
				"stroke_color",
				t("ui.editor.shape.stroke_color"),
				item.stroke_color ?? "#000000",
			)}
			{number(
				"stroke_width",
				t("ui.editor.shape.stroke_width"),
				0,
				item.stroke_width ?? 0,
			)}
		</Section>
	);
}
