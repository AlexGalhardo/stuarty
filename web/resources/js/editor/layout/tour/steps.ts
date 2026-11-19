// The steps of the first-run tour (SDD-012 section 18.5), in the order a person
// meets the editor: library, preview, timeline, inspector, export. `target` is
// the selector of the region the step outlines and `placement` says on which
// side of it the card sits; a region that is not on screen (a closed panel)
// leaves the card at the bottom centre with no outline.
export type TourPlacement = "right" | "inside" | "above" | "left" | "below";

export interface TourStep {
	id: "library" | "preview" | "timeline" | "inspector" | "export";
	target: string;
	placement: TourPlacement;
}

export const TOUR_STEPS: readonly TourStep[] = [
	{ id: "library", target: '[data-landmark="2"]', placement: "right" },
	{ id: "preview", target: '[data-landmark="3"]', placement: "inside" },
	{ id: "timeline", target: '[data-landmark="4"]', placement: "above" },
	{ id: "inspector", target: '[data-landmark="5"]', placement: "left" },
	{ id: "export", target: '[data-tour="export"]', placement: "below" },
];
