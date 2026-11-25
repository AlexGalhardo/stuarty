import { isTheme, type Theme } from "./dayTypes";

// What a theme looks like: the soft surface of a family of the design system and its text, the pairs that
// `check:contrast` measures (docs/design-system.md section 2.6), so a theme never needs a colour of its own. `swatch`
// is the solid colour of the small round chip that picks it, drawn beside a word, never alone.
export const THEME_CLASSES: Record<
	Theme,
	{ surface: string; swatch: string; ring: string }
> = {
	blue: {
		surface: "bg-accent-soft text-accent-text",
		swatch: "bg-accent",
		ring: "ring-accent",
	},
	green: {
		surface: "bg-success-soft text-success-text",
		swatch: "bg-success",
		ring: "ring-success",
	},
	amber: {
		surface: "bg-warning-soft text-warning-text",
		swatch: "bg-warning",
		ring: "ring-warning",
	},
	red: {
		surface: "bg-danger-soft text-danger-text",
		swatch: "bg-danger",
		ring: "ring-danger",
	},
	teal: {
		surface: "bg-agent-soft text-agent-text",
		swatch: "bg-agent",
		ring: "ring-agent",
	},
	gray: {
		surface: "bg-surface-sunken text-text",
		swatch: "bg-border-strong",
		ring: "ring-border-strong",
	},
};

// The classes of a theme the service named; none for a day with no theme or one this build does not know.
export function themeSurface(theme: string | undefined): string {
	return isTheme(theme) ? THEME_CLASSES[theme].surface : "";
}
