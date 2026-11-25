import type { CSSProperties } from "react";

import registry from "@/generated/commands.json";
import type { CaptionStyle } from "@/generated/timeline";

// The named caption styles of the core (SDD-002 C.88) as the registry carries them: the `x-vv-caption-styles`
// extension of the `style` argument of caption.apply_style. The gallery is drawn from this table and holds no
// style of its own; applying one sends its id and the core resolves it.

export interface CaptionStyleEntry {
	readonly id: string;
	readonly name: string;
	readonly summary: string;
	readonly font_family: string;
	// The stored fields for a frame whose shorter side is 1080 pixels.
	readonly caption_style: CaptionStyle;
}

interface Schema {
	readonly properties?: Readonly<Record<string, Schema>>;
	readonly "x-vv-caption-styles"?: readonly CaptionStyleEntry[];
}

const applyStyle = registry.find((c) => c.name === "caption.apply_style")
	?.args_schema as unknown as Schema | undefined;

export const CAPTION_STYLES: readonly CaptionStyleEntry[] =
	applyStyle?.properties?.style?.["x-vv-caption-styles"] ?? [];

// The height of the sample text in the gallery, in CSS pixels.
const SAMPLE_PX = 15;

// A sample of a style for a tile: the colours, the edge, the shadow and the box of the style at a size that
// fits the tile, so that every length keeps its proportion to the text. It draws what the core stores; the
// frame the person gets is the one the renderer composes. The font is the family named by the style when
// the browser has it; the browser never loads a font file, so the tile also shows the family by name.
export function captionStyleSample(style: CaptionStyle): CSSProperties {
	const scale = SAMPLE_PX / (style.size ?? SAMPLE_PX);
	const px = (value: number | undefined): number => (value ?? 0) * scale;
	const css: CSSProperties = {
		fontFamily: `"${style.font_family ?? "Inter"}", sans-serif`,
		fontWeight: style.font_weight ?? 700,
		fontSize: `${SAMPLE_PX}px`,
		lineHeight: 1.1,
		color: style.color,
		textTransform: style.uppercase === true ? "uppercase" : "none",
		textAlign: style.align ?? "center",
	};
	if (style.stroke_color !== undefined && style.stroke_width !== undefined) {
		css.WebkitTextStroke = `${px(style.stroke_width)}px ${style.stroke_color}`;
		css.paintOrder = "stroke fill";
	}
	if (style.shadow_color !== undefined) {
		css.textShadow = `${px(style.shadow_dx)}px ${px(style.shadow_dy)}px ${px(style.shadow_blur)}px ${style.shadow_color}`;
	}
	if (style.background_color !== undefined) {
		css.backgroundColor = style.background_color;
		css.padding = `${px(style.background_padding) / 2}px ${px(style.background_padding)}px`;
		css.borderRadius = `${px(style.background_radius)}px`;
	}
	return css;
}
