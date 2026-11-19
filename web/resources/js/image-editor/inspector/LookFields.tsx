import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type {
	CurvePoint,
	Filter,
	FilterKind,
	ImageDocument,
	ImageOperation,
	Layer,
	Mask,
	MaskKind,
} from "../api/types";
import { useImage } from "../state/context";
import {
	ColorField,
	NumberField,
	Section,
	SelectField,
	TextField,
	ToggleField,
} from "./fields";

// The look of a layer in the inspector (SDD-021 section 3.1, epic 29): adjustments, filters, a mask and a blend
// mode. Each field sends one command of the catalogue and shows what the server answers; no pixel and no limit
// is decided here (SDD-012 section 1): the parameters below only say which fields a kind of filter has.

export const BLEND_MODES = [
	"normal",
	"multiply",
	"screen",
	"overlay",
	"soft_light",
	"hard_light",
	"darken",
	"lighten",
	"color_dodge",
	"color_burn",
	"difference",
] as const;

export const FILTER_KINDS: readonly FilterKind[] = [
	"grayscale",
	"sepia",
	"invert",
	"warm",
	"cool",
	"blur",
	"sharpen",
	"vignette",
	"grain",
	"shadow",
	"glow",
	"stroke",
];

type NumberParam =
	| "amount"
	| "radius"
	| "softness"
	| "size"
	| "seed"
	| "opacity"
	| "dx"
	| "dy"
	| "blur"
	| "width";

// The members each kind of filter has, with the unit its field shows.
const PARAMS: Record<FilterKind, { key: NumberParam; unit: string }[]> = {
	grayscale: [{ key: "amount", unit: "%" }],
	sepia: [{ key: "amount", unit: "%" }],
	invert: [{ key: "amount", unit: "%" }],
	warm: [{ key: "amount", unit: "%" }],
	cool: [{ key: "amount", unit: "%" }],
	blur: [{ key: "radius", unit: "px" }],
	sharpen: [
		{ key: "amount", unit: "%" },
		{ key: "radius", unit: "px" },
	],
	vignette: [
		{ key: "amount", unit: "%" },
		{ key: "softness", unit: "%" },
	],
	grain: [
		{ key: "amount", unit: "%" },
		{ key: "size", unit: "px" },
		{ key: "seed", unit: "" },
	],
	shadow: [
		{ key: "opacity", unit: "%" },
		{ key: "dx", unit: "px" },
		{ key: "dy", unit: "px" },
		{ key: "blur", unit: "px" },
	],
	glow: [
		{ key: "radius", unit: "px" },
		{ key: "amount", unit: "%" },
	],
	stroke: [{ key: "width", unit: "px" }],
};

const HAS_COLOUR: ReadonlySet<FilterKind> = new Set([
	"shadow",
	"glow",
	"stroke",
]);

export default function LookFields({
	doc,
	layer,
}: {
	doc: ImageDocument;
	layer: Layer;
}): ReactNode {
	return (
		<>
			<AdjustFields layer={layer} />
			<FilterFields layer={layer} />
			<MaskFields doc={doc} layer={layer} />
			<BlendField layer={layer} />
		</>
	);
}

const ADJUST_KEYS = [
	"brightness",
	"contrast",
	"saturation",
	"temperature",
	"tint",
	"hue",
	"exposure",
] as const;

// "64:48, 192:208" as the points of a curve; null when the text is not a list of such pairs.
export function parseCurve(text: string): CurvePoint[] | null {
	const body = text.trim();
	if (body === "") return [];
	const points: CurvePoint[] = [];
	for (const part of body.split(",")) {
		const m = /^\s*(\d{1,3})\s*[:;]\s*(\d{1,3})\s*$/.exec(part);
		if (m === null) return null;
		points.push({ x: Number(m[1]), y: Number(m[2]) });
	}
	return points;
}

export function curveText(points: readonly CurvePoint[] | undefined): string {
	return (points ?? []).map((p) => `${p.x}:${p.y}`).join(", ");
}

function AdjustFields({ layer }: { layer: Layer }): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const locked = layer.locked === true;
	const adjust = layer.adjust ?? {};
	const levels = adjust.levels ?? { black: 0, white: 255, gamma: 100 };
	const run = (args: { [key: string]: unknown }) =>
		send([{ op: "layer.adjust", args: { layer_id: layer.id, ...args } }]);

	return (
		<Section title={t("ui.image.look.adjust")} testId="image-adjust-fields">
			<div className="grid grid-cols-2 gap-2">
				{ADJUST_KEYS.map((key) => (
					<NumberField
						key={key}
						label={t(`ui.image.look.${key}`)}
						value={adjust[key] ?? 0}
						unit={key === "hue" ? "°" : undefined}
						disabled={locked}
						testId={`image-adjust-${key}`}
						onCommit={(value) => run({ [key]: value })}
					/>
				))}
			</div>
			<p className="text-xs font-medium">{t("ui.image.look.levels")}</p>
			<div className="grid grid-cols-3 gap-2">
				<NumberField
					label={t("ui.image.look.levels_black")}
					value={levels.black}
					disabled={locked}
					testId="image-adjust-levels-black"
					onCommit={(black) => run({ levels: { black } })}
				/>
				<NumberField
					label={t("ui.image.look.levels_white")}
					value={levels.white}
					disabled={locked}
					testId="image-adjust-levels-white"
					onCommit={(white) => run({ levels: { white } })}
				/>
				<NumberField
					label={t("ui.image.look.levels_gamma")}
					value={levels.gamma}
					disabled={locked}
					testId="image-adjust-levels-gamma"
					onCommit={(gamma) => run({ levels: { gamma } })}
				/>
			</div>
			<TextField
				label={t("ui.image.look.curve")}
				value={curveText(adjust.curve)}
				disabled={locked}
				testId="image-adjust-curve"
				onCommit={(text) => {
					const curve = parseCurve(text);
					// A text that is not a list of points is not sent; null puts the shown value back.
					return curve === null ? Promise.resolve(null) : run({ curve });
				}}
			/>
			<p className="text-xs text-muted-foreground">
				{t("ui.image.look.curve_hint")}
			</p>
			<div>
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={locked || layer.adjust === undefined}
					data-testid="image-adjust-reset"
					onClick={() => void run({ reset: true })}
				>
					{t("ui.image.look.adjust_reset")}
				</Button>
			</div>
		</Section>
	);
}

function FilterFields({ layer }: { layer: Layer }): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const locked = layer.locked === true;
	const filters = layer.filters ?? [];
	const free = FILTER_KINDS.filter((k) => !filters.some((f) => f.kind === k));
	const [kind, setKind] = useState<FilterKind | "">("");
	const chosen: FilterKind | "" = free.some((k) => k === kind)
		? kind
		: (free[0] ?? "");
	const run = (op: string, args: { [key: string]: unknown }) =>
		send([{ op, args: { layer_id: layer.id, ...args } }]);

	return (
		<Section title={t("ui.image.look.filters")} testId="image-filter-fields">
			{filters.length === 0 && (
				<p className="text-xs text-muted-foreground">
					{t("ui.image.look.filters_none")}
				</p>
			)}
			{filters.map((filter, index) => (
				<fieldset
					key={filter.kind}
					data-testid={`image-filter-${filter.kind}`}
					className="m-0 flex min-w-0 flex-col gap-2 rounded-lg border p-2"
				>
					<legend className="px-1 text-xs font-medium">
						{t(`ui.image.look.filter_${filter.kind}`)}
					</legend>
					<FilterParams
						layer={layer}
						filter={filter}
						locked={locked}
						onSet={(args) =>
							run("layer.filter_set", { kind: filter.kind, ...args })
						}
					/>
					<div className="flex gap-1">
						<Button
							type="button"
							variant="outline"
							size="sm"
							disabled={locked || index === 0}
							aria-label={t("ui.image.look.filter_up", {
								name: t(`ui.image.look.filter_${filter.kind}`),
							})}
							data-testid={`image-filter-${filter.kind}-up`}
							onClick={() =>
								void run("layer.filter_move", {
									kind: filter.kind,
									index: index - 1,
								})
							}
						>
							{t("ui.image.look.move_up")}
						</Button>
						<Button
							type="button"
							variant="outline"
							size="sm"
							disabled={locked || index === filters.length - 1}
							aria-label={t("ui.image.look.filter_down", {
								name: t(`ui.image.look.filter_${filter.kind}`),
							})}
							data-testid={`image-filter-${filter.kind}-down`}
							onClick={() =>
								void run("layer.filter_move", {
									kind: filter.kind,
									index: index + 1,
								})
							}
						>
							{t("ui.image.look.move_down")}
						</Button>
						<Button
							type="button"
							variant="outline"
							size="sm"
							disabled={locked}
							aria-label={t("ui.image.look.filter_remove", {
								name: t(`ui.image.look.filter_${filter.kind}`),
							})}
							data-testid={`image-filter-${filter.kind}-remove`}
							onClick={() =>
								void run("layer.filter_remove", { kind: filter.kind })
							}
						>
							{t("ui.image.look.remove")}
						</Button>
					</div>
				</fieldset>
			))}
			{free.length > 0 && (
				<div className="flex items-end gap-1">
					<div className="min-w-0 flex-1">
						<SelectField
							label={t("ui.image.look.filter_add_kind")}
							value={chosen}
							disabled={locked}
							testId="image-filter-add-kind"
							options={free.map((k) => ({
								value: k,
								label: t(`ui.image.look.filter_${k}`),
							}))}
							onCommit={(value) => {
								setKind(value as FilterKind);
								return undefined;
							}}
						/>
					</div>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={locked || chosen === ""}
						data-testid="image-filter-add"
						onClick={() => {
							if (chosen !== "") void run("layer.filter_add", { kind: chosen });
						}}
					>
						{t("ui.image.look.filter_add")}
					</Button>
				</div>
			)}
		</Section>
	);
}

function FilterParams({
	layer,
	filter,
	locked,
	onSet,
}: {
	layer: Layer;
	filter: Filter;
	locked: boolean;
	onSet: (args: { [key: string]: unknown }) => undefined | Promise<unknown>;
}): ReactNode {
	const t = useT();
	return (
		<div className="grid grid-cols-2 gap-2">
			{PARAMS[filter.kind].map(({ key, unit }) => (
				<NumberField
					key={`${layer.id}-${key}`}
					label={t(`ui.image.look.param_${key}`)}
					value={filter[key] ?? 0}
					unit={unit === "" ? undefined : unit}
					disabled={locked}
					testId={`image-filter-${filter.kind}-${key}`}
					onCommit={(value) => onSet({ [key]: value })}
				/>
			))}
			{HAS_COLOUR.has(filter.kind) && (
				<ColorField
					label={t("ui.image.look.param_color")}
					value={filter.color ?? "#000000"}
					disabled={locked}
					testId={`image-filter-${filter.kind}-color`}
					onCommit={(color) => onSet({ color })}
				/>
			)}
		</div>
	);
}

const MASK_KINDS: readonly (MaskKind | "none")[] = [
	"none",
	"rect",
	"rounded_rect",
	"ellipse",
	"layer",
];

function flatten(layers: readonly Layer[]): Layer[] {
	return layers.flatMap((l) => [l, ...flatten(l.children ?? [])]);
}

// The command a field of a shape mask sends. layer.mask_set replaces the whole mask, so the members the field
// does not change are read from `doc`, the document when the command's turn comes in the queue, and not from
// the render the field belongs to: two fields committed before the server answers the first would otherwise
// send the older mask back and undo the first change.
export function maskChange(
	doc: ImageDocument,
	layerId: string,
	patch: Partial<Mask>,
): ImageOperation[] {
	const mask = flatten(doc.layers).find((l) => l.id === layerId)?.mask;
	return [
		{
			op: "layer.mask_set",
			args: { layer_id: layerId, ...shapeArgs(mask), ...patch },
		},
	];
}

function MaskFields({
	doc,
	layer,
}: {
	doc: ImageDocument;
	layer: Layer;
}): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	const locked = layer.locked === true;
	const mask = layer.mask;
	const kind: MaskKind | "none" = mask?.kind ?? "none";
	// The layers a mask can take its shape from: any other that is not a group.
	const sources = flatten(doc.layers).filter(
		(l) => l.id !== layer.id && l.kind !== "group",
	);
	const set = (args: { [key: string]: unknown }) =>
		send([{ op: "layer.mask_set", args: { layer_id: layer.id, ...args } }]);
	const current = (d: ImageDocument): Mask | undefined =>
		flatten(d.layers).find((l) => l.id === layer.id)?.mask;
	const change = (patch: Partial<Mask>) =>
		send((d) => maskChange(d, layer.id, patch));

	return (
		<Section title={t("ui.image.look.mask")} testId="image-mask-fields">
			<SelectField
				label={t("ui.image.look.mask_kind")}
				value={kind}
				disabled={locked}
				testId="image-mask-kind"
				options={MASK_KINDS.map((k) => ({
					value: k,
					label: t(`ui.image.look.mask_${k}`),
				}))}
				onCommit={(value) => {
					if (value === "none") {
						return send([
							{ op: "layer.mask_clear", args: { layer_id: layer.id } },
						]);
					}
					if (value === "layer") {
						const first = sources[0];
						return first === undefined
							? Promise.resolve(null)
							: set({ kind: "layer", source: first.id });
					}
					return set({ kind: value });
				}}
			/>
			{mask !== undefined && mask.kind !== "layer" && (
				<>
					<div className="grid grid-cols-2 gap-2">
						{(["x", "y", "w", "h"] as const).map((key) => (
							<NumberField
								key={key}
								label={t(`ui.image.look.mask_${key}`)}
								value={mask[key] ?? (key === "w" || key === "h" ? 1000 : 0)}
								unit="‰"
								disabled={locked}
								testId={`image-mask-${key}`}
								onCommit={(value) => change({ kind: mask.kind, [key]: value })}
							/>
						))}
						{mask.kind === "rounded_rect" && (
							<NumberField
								label={t("ui.image.look.mask_radius")}
								value={mask.radius ?? 0}
								unit="px"
								disabled={locked}
								testId="image-mask-radius"
								onCommit={(radius) => change({ kind: mask.kind, radius })}
							/>
						)}
						<NumberField
							label={t("ui.image.look.mask_feather")}
							value={mask.feather ?? 0}
							unit="px"
							disabled={locked}
							testId="image-mask-feather"
							onCommit={(feather) => change({ kind: mask.kind, feather })}
						/>
					</div>
					<ToggleField
						label={t("ui.image.look.mask_invert")}
						checked={mask.invert === true}
						disabled={locked}
						testId="image-mask-invert"
						onCommit={(invert) => change({ kind: mask.kind, invert })}
					/>
				</>
			)}
			{mask !== undefined && mask.kind === "layer" && (
				<>
					<SelectField
						label={t("ui.image.look.mask_source")}
						value={mask.source ?? ""}
						disabled={locked}
						testId="image-mask-source"
						options={sources.map((l) => ({ value: l.id, label: l.name }))}
						onCommit={(source) =>
							send((d) => [
								{
									op: "layer.mask_set",
									args: {
										layer_id: layer.id,
										kind: "layer",
										source,
										invert: current(d)?.invert === true,
									},
								},
							])
						}
					/>
					<ToggleField
						label={t("ui.image.look.mask_invert")}
						checked={mask.invert === true}
						disabled={locked}
						testId="image-mask-invert"
						onCommit={(invert) =>
							send((d) => [
								{
									op: "layer.mask_set",
									args: {
										layer_id: layer.id,
										kind: "layer",
										source: current(d)?.source ?? mask.source,
										invert,
									},
								},
							])
						}
					/>
				</>
			)}
		</Section>
	);
}

// What layer.mask_set needs to keep the members of a shape mask the field does not change.
function shapeArgs(mask: Mask | undefined): { [key: string]: unknown } {
	if (mask === undefined || mask.kind === "layer") return {};
	const args: { [key: string]: unknown } = {};
	for (const key of ["x", "y", "w", "h", "radius", "feather"] as const) {
		const value = mask[key];
		if (value !== undefined) args[key] = value;
	}
	if (mask.invert === true) args.invert = true;
	return args;
}

function BlendField({ layer }: { layer: Layer }): ReactNode {
	const t = useT();
	const send = useImage((s) => s.send);
	return (
		<Section title={t("ui.image.look.blend")} testId="image-blend-fields">
			<SelectField
				label={t("ui.image.look.blend_mode")}
				value={layer.blend ?? "normal"}
				disabled={layer.locked === true}
				testId="image-blend-mode"
				options={BLEND_MODES.map((mode) => ({
					value: mode,
					label: t(`ui.image.look.blend_${mode}`),
				}))}
				onCommit={(mode) =>
					send([{ op: "layer.blend", args: { layer_id: layer.id, mode } }])
				}
			/>
		</Section>
	);
}
