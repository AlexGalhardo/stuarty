import {
	AlignCenterHorizontal,
	AlignCenterVertical,
	AlignEndHorizontal,
	AlignEndVertical,
	AlignStartHorizontal,
	AlignStartVertical,
} from "lucide-react";
import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { ImageDocument, Layer } from "../api/types";
import { findLayer, useImage } from "../state/context";
import { useLibrary } from "../state/library";
import ToolButton from "../toolbar/ToolButton";
import {
	ColorField,
	NumberField,
	Section,
	SelectField,
	TextField,
	ToggleField,
} from "./fields";
import { replacePhoto } from "./replace";

const WEIGHTS = [100, 200, 300, 400, 500, 600, 700, 800, 900];

// The inspector (SDD-012 section 24.6): the fields of the document, or of the layer selected, by kind. A field
// sends one command of SDD-021 section 3 and the server's answer is what it shows next.
export default function Inspector(): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const selection = useImage((s) => s.selection);
	if (doc === null) return null;
	const layers = selection
		.map((id) => findLayer(doc.layers, id))
		.filter((l): l is Layer => l !== undefined);

	return (
		<div
			data-testid="image-inspector"
			className="min-h-0 flex-1 overflow-y-auto"
		>
			{layers.length === 0 && <DocumentFields doc={doc} />}
			{layers.length > 1 && (
				<>
					<Section title={t("ui.image.inspector.selection")}>
						<p className="text-sm">
							{t("ui.image.canvas.many_selected", { count: layers.length })}
						</p>
					</Section>
					<AlignFields doc={doc} layers={layers} />
				</>
			)}
			{layers.length === 1 && layers[0] !== undefined && (
				<LayerFields doc={doc} layer={layers[0]} />
			)}
		</div>
	);
}

function DocumentFields({ doc }: { doc: ImageDocument }): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const [mode, setMode] = useState("keep");
	const resize = (width: number, height: number) =>
		send([{ op: "canvas.resize", args: { width, height, mode } }]);

	return (
		<Section title={t("ui.image.inspector.document")} testId="image-doc-fields">
			<div className="grid grid-cols-2 gap-2">
				<NumberField
					label={t("ui.image.inspector.canvas_width")}
					value={doc.width}
					unit="px"
					testId="image-canvas-width"
					onCommit={(w) => resize(w, doc.height)}
				/>
				<NumberField
					label={t("ui.image.inspector.canvas_height")}
					value={doc.height}
					unit="px"
					testId="image-canvas-height"
					onCommit={(h) => resize(doc.width, h)}
				/>
			</div>
			<SelectField
				label={t("ui.image.inspector.resize_mode")}
				value={mode}
				testId="image-resize-mode"
				options={[
					{ value: "keep", label: t("ui.image.inspector.resize_keep") },
					{ value: "scale", label: t("ui.image.inspector.resize_scale") },
				]}
				onCommit={(value) => {
					setMode(value);
					return undefined;
				}}
			/>
			<p className="text-xs text-muted-foreground">
				{t("ui.image.inspector.document_hint")}
			</p>
		</Section>
	);
}

function AlignFields({
	doc,
	layers,
}: {
	doc: ImageDocument;
	layers: readonly Layer[];
}): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const ids = layers.map((l) => l.id);
	const [to, setTo] = useState("canvas");
	const others = flatten(doc.layers).filter((l) => !ids.includes(l.id));
	const target =
		to === "canvas" || others.some((l) => l.id === to) ? to : "canvas";
	const align = (args: { horizontal?: string; vertical?: string }) =>
		void send([
			{ op: "layer.align", args: { layer_ids: ids, to: target, ...args } },
		]);

	return (
		<Section title={t("ui.image.inspector.align")} testId="image-align">
			<div
				className="flex flex-wrap gap-1"
				role="toolbar"
				aria-label={t("ui.image.inspector.align")}
			>
				<ToolButton
					icon={AlignStartHorizontal}
					label={t("ui.image.align.left")}
					onClick={() => align({ horizontal: "left" })}
					testId="image-align-left"
				/>
				<ToolButton
					icon={AlignCenterHorizontal}
					label={t("ui.image.align.center")}
					onClick={() => align({ horizontal: "center" })}
					testId="image-align-center"
				/>
				<ToolButton
					icon={AlignEndHorizontal}
					label={t("ui.image.align.right")}
					onClick={() => align({ horizontal: "right" })}
					testId="image-align-right"
				/>
				<ToolButton
					icon={AlignStartVertical}
					label={t("ui.image.align.top")}
					onClick={() => align({ vertical: "top" })}
					testId="image-align-top"
				/>
				<ToolButton
					icon={AlignCenterVertical}
					label={t("ui.image.align.middle")}
					onClick={() => align({ vertical: "middle" })}
					testId="image-align-middle"
				/>
				<ToolButton
					icon={AlignEndVertical}
					label={t("ui.image.align.bottom")}
					onClick={() => align({ vertical: "bottom" })}
					testId="image-align-bottom"
				/>
			</div>
			<SelectField
				label={t("ui.image.inspector.align_to")}
				value={target}
				testId="image-align-to"
				options={[
					{ value: "canvas", label: t("ui.image.inspector.canvas") },
					...others.map((l) => ({ value: l.id, label: l.name })),
				]}
				onCommit={(value) => {
					setTo(value);
					return undefined;
				}}
			/>
		</Section>
	);
}

function flatten(layers: readonly Layer[]): Layer[] {
	return layers.flatMap((l) => [l, ...flatten(l.children ?? [])]);
}

function LayerFields({
	doc,
	layer,
}: {
	doc: ImageDocument;
	layer: Layer;
}): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const locked = layer.locked === true;
	const group = layer.kind === "group";
	const set = (args: { [key: string]: unknown }) =>
		send([{ op: "layer.set", args: { layer_id: layer.id, ...args } }]);
	const transform = (args: { [key: string]: unknown }) =>
		send([{ op: "layer.transform", args: { layer_id: layer.id, ...args } }]);

	return (
		<>
			<Section
				title={t("ui.image.inspector.layer")}
				testId="image-layer-fields"
			>
				<TextField
					label={t("ui.image.inspector.name")}
					value={layer.name}
					maxLength={120}
					testId="image-field-name"
					onCommit={(name) => set({ name })}
				/>
				<NumberField
					label={t("ui.image.inspector.opacity")}
					value={layer.opacity}
					axis={100}
					unit="%"
					disabled={locked}
					testId="image-field-opacity"
					onCommit={(opacity) => set({ opacity })}
				/>
				<div className="flex gap-4">
					<ToggleField
						label={t("ui.image.inspector.visible")}
						checked={layer.visible}
						testId="image-field-visible"
						onCommit={(visible) => set({ visible })}
					/>
					<ToggleField
						label={t("ui.image.inspector.locked")}
						checked={locked}
						testId="image-field-locked"
						onCommit={(value) => set({ locked: value })}
					/>
				</div>
				{(layer.kind === "text" || layer.kind === "image") && (
					<TextField
						label={t("ui.image.inspector.slot")}
						value={layer.slot ?? ""}
						disabled={false}
						testId="image-field-slot"
						onCommit={(slot) => set({ slot })}
					/>
				)}
			</Section>
			<Section title={t("ui.image.inspector.box")} testId="image-box-fields">
				<div className="grid grid-cols-2 gap-2">
					<NumberField
						label={t("ui.image.inspector.x")}
						value={layer.x}
						axis={doc.width}
						unit="px"
						disabled={locked}
						testId="image-field-x"
						onCommit={(x) => transform({ x })}
					/>
					<NumberField
						label={t("ui.image.inspector.y")}
						value={layer.y}
						axis={doc.height}
						unit="px"
						disabled={locked}
						testId="image-field-y"
						onCommit={(y) => transform({ y })}
					/>
					{!group && (
						<>
							<NumberField
								label={t("ui.image.inspector.width")}
								value={layer.w}
								axis={doc.width}
								unit="px"
								disabled={locked}
								testId="image-field-w"
								onCommit={(w) => transform({ w })}
							/>
							<NumberField
								label={t("ui.image.inspector.height")}
								value={layer.h}
								axis={doc.height}
								unit="px"
								disabled={locked}
								testId="image-field-h"
								onCommit={(h) => transform({ h })}
							/>
							<NumberField
								label={t("ui.image.inspector.rotation")}
								value={layer.rotation ?? 0}
								unit="°"
								disabled={locked}
								testId="image-field-rotation"
								onCommit={(rotation) => transform({ rotation })}
							/>
						</>
					)}
				</div>
			</Section>
			<AlignFields doc={doc} layers={[layer]} />
			{layer.kind === "image" && <ImageFields doc={doc} layer={layer} />}
			{layer.kind === "text" && <TextFields layer={layer} />}
			{(layer.kind === "shape" || layer.kind === "fill") && (
				<ShapeFields layer={layer} />
			)}
		</>
	);
}

function ImageFields({
	doc,
	layer,
}: {
	doc: ImageDocument;
	layer: Layer;
}): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const library = useLibrary();
	const locked = layer.locked === true;
	// No crop means the whole source: the fields start from it, as the library knows its size.
	const src = library.images.find((a) => a.hash === layer.asset);
	const crop =
		layer.crop ??
		(src?.width && src.height
			? { x: 0, y: 0, w: src.width, h: src.height }
			: undefined);
	const source = layer.asset ?? "";
	const setCrop = (patch: { [key: string]: number }) =>
		crop === undefined
			? undefined
			: send([
					{
						op: "layer.crop",
						args: { layer_id: layer.id, ...crop, ...patch },
					},
				]);
	const fitTo = (mode: "cover" | "contain") =>
		void send([{ op: "layer.fit", args: { layer_id: layer.id, mode } }]);

	return (
		<Section title={t("ui.image.inspector.image")} testId="image-image-fields">
			<SelectField
				label={t("ui.image.inspector.replace")}
				value={source}
				disabled={locked}
				testId="image-field-asset"
				options={[
					...(library.images.some((a) => a.hash === source)
						? []
						: [
								{ value: source, label: t("ui.image.inspector.current_image") },
							]),
					...library.images.map((a) => ({ value: a.hash, label: a.name })),
				]}
				onCommit={(hash) =>
					hash === source ? undefined : replacePhoto(send, doc, layer, hash)
				}
			/>
			<div className="flex gap-1">
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={locked}
					data-testid="image-fit-cover"
					onClick={() => fitTo("cover")}
				>
					{t("ui.image.inspector.fit_cover")}
				</Button>
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={locked}
					data-testid="image-fit-contain"
					onClick={() => fitTo("contain")}
				>
					{t("ui.image.inspector.fit_contain")}
				</Button>
			</div>
			{crop !== undefined && (
				<>
					<p className="text-xs font-medium">{t("ui.image.inspector.crop")}</p>
					<div className="grid grid-cols-2 gap-2">
						<NumberField
							label={t("ui.image.inspector.x")}
							value={crop.x}
							unit="px"
							disabled={locked}
							testId="image-crop-x"
							onCommit={(x) => setCrop({ x })}
						/>
						<NumberField
							label={t("ui.image.inspector.y")}
							value={crop.y}
							unit="px"
							disabled={locked}
							testId="image-crop-y"
							onCommit={(y) => setCrop({ y })}
						/>
						<NumberField
							label={t("ui.image.inspector.width")}
							value={crop.w}
							unit="px"
							disabled={locked}
							testId="image-crop-w"
							onCommit={(w) => setCrop({ w })}
						/>
						<NumberField
							label={t("ui.image.inspector.height")}
							value={crop.h}
							unit="px"
							disabled={locked}
							testId="image-crop-h"
							onCommit={(h) => setCrop({ h })}
						/>
					</div>
				</>
			)}
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={locked}
				data-testid="image-crop-reset"
				onClick={() =>
					void send([
						{ op: "layer.crop", args: { layer_id: layer.id, reset: true } },
					])
				}
			>
				{t("ui.image.inspector.crop_reset")}
			</Button>
		</Section>
	);
}

function TextFields({ layer }: { layer: Layer }): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const library = useLibrary();
	const locked = layer.locked === true;
	const style = layer.style ?? {};
	const swatches = Object.entries(library.brand?.colors ?? {}).map(
		([name, value]) => ({ name, value }),
	);
	const families = [
		...new Set([
			...(style.font_family ? [style.font_family] : []),
			...Object.keys(library.brand?.fonts ?? {}),
			...library.fonts.map((f) => f.family),
		]),
	];
	const setStyle = (patch: { [key: string]: unknown }) =>
		send([
			{ op: "layer.set_text", args: { layer_id: layer.id, style: patch } },
		]);

	return (
		<Section title={t("ui.image.inspector.text")} testId="image-text-fields">
			<TextField
				label={t("ui.image.inspector.content")}
				value={layer.text ?? ""}
				multiline
				disabled={locked}
				maxLength={2000}
				testId="image-field-text"
				onCommit={(text) =>
					send([{ op: "layer.set_text", args: { layer_id: layer.id, text } }])
				}
			/>
			<SelectField
				label={t("ui.image.inspector.font")}
				value={style.font_family ?? ""}
				disabled={locked}
				testId="image-field-font"
				options={[
					...(style.font_family
						? []
						: [{ value: "", label: t("ui.image.inspector.font_default") }]),
					...families.map((family) => ({ value: family, label: family })),
				]}
				onCommit={(font_family) =>
					font_family === "" ? undefined : setStyle({ font_family })
				}
			/>
			<div className="grid grid-cols-2 gap-2">
				<SelectField
					label={t("ui.image.inspector.weight")}
					value={String(style.font_weight ?? 400)}
					disabled={locked}
					testId="image-field-weight"
					options={WEIGHTS.map((w) => ({ value: String(w), label: String(w) }))}
					onCommit={(w) => setStyle({ font_weight: Number(w) })}
				/>
				<NumberField
					label={t("ui.image.inspector.size")}
					value={style.size ?? 48}
					unit="px"
					disabled={locked}
					testId="image-field-size"
					onCommit={(size) => setStyle({ size })}
				/>
			</div>
			<ColorField
				label={t("ui.image.inspector.color")}
				value={style.color ?? "#FFFFFF"}
				swatches={swatches}
				disabled={locked}
				testId="image-field-color"
				onCommit={(color) => setStyle({ color })}
			/>
			<SelectField
				label={t("ui.image.inspector.align_text")}
				value={style.align ?? "left"}
				disabled={locked}
				testId="image-field-text-align"
				options={[
					{ value: "left", label: t("ui.image.align.left") },
					{ value: "center", label: t("ui.image.align.center") },
					{ value: "right", label: t("ui.image.align.right") },
				]}
				onCommit={(align) => setStyle({ align })}
			/>
			<ToggleField
				label={t("ui.image.inspector.uppercase")}
				checked={style.uppercase === true}
				disabled={locked}
				testId="image-field-uppercase"
				onCommit={(uppercase) => setStyle({ uppercase })}
			/>
			<ColorField
				label={t("ui.image.inspector.stroke_color")}
				value={style.stroke_color ?? ""}
				swatches={swatches}
				allowEmpty
				disabled={locked}
				testId="image-field-text-stroke"
				onCommit={(stroke_color) =>
					setStyle({ stroke_color: stroke_color === "" ? null : stroke_color })
				}
			/>
			<NumberField
				label={t("ui.image.inspector.stroke_width")}
				value={style.stroke_width ?? 0}
				unit="px"
				disabled={locked}
				testId="image-field-text-stroke-width"
				onCommit={(stroke_width) => setStyle({ stroke_width })}
			/>
		</Section>
	);
}

function ShapeFields({ layer }: { layer: Layer }): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const library = useLibrary();
	const locked = layer.locked === true;
	const swatches = Object.entries(library.brand?.colors ?? {}).map(
		([name, value]) => ({ name, value }),
	);
	const set = (args: { [key: string]: unknown }) =>
		send([{ op: "layer.set_shape", args: { layer_id: layer.id, ...args } }]);

	return (
		<Section
			title={t(
				layer.kind === "fill"
					? "ui.image.kind.fill"
					: "ui.image.inspector.shape",
			)}
			testId="image-shape-fields"
		>
			{layer.kind === "shape" && (
				<SelectField
					label={t("ui.image.inspector.shape_kind")}
					value={layer.shape ?? "rect"}
					disabled={locked}
					testId="image-field-shape"
					options={[
						{ value: "rect", label: t("ui.image.shape.rect") },
						{ value: "rounded_rect", label: t("ui.image.shape.rounded_rect") },
						{ value: "ellipse", label: t("ui.image.shape.ellipse") },
					]}
					onCommit={(shape) => set({ shape })}
				/>
			)}
			<ColorField
				label={t("ui.image.inspector.fill")}
				value={layer.fill ?? "#FFFFFF"}
				swatches={swatches}
				disabled={locked}
				testId="image-field-fill"
				onCommit={(fill) => set({ fill })}
			/>
			{layer.kind === "shape" && (
				<>
					<ColorField
						label={t("ui.image.inspector.stroke_color")}
						value={layer.stroke_color ?? ""}
						swatches={swatches}
						allowEmpty
						disabled={locked}
						testId="image-field-stroke"
						onCommit={(stroke_color) => set({ stroke_color })}
					/>
					<NumberField
						label={t("ui.image.inspector.stroke_width")}
						value={layer.stroke_width ?? 0}
						unit="px"
						disabled={locked}
						testId="image-field-stroke-width"
						onCommit={(stroke_width) => set({ stroke_width })}
					/>
					{layer.shape === "rounded_rect" && (
						<NumberField
							label={t("ui.image.inspector.radius")}
							value={layer.radius ?? 0}
							unit="px"
							disabled={locked}
							testId="image-field-radius"
							onCommit={(radius) => set({ radius })}
						/>
					)}
				</>
			)}
		</Section>
	);
}
