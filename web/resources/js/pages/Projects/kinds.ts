import {
	Clapperboard,
	FileText,
	ImageIcon,
	type LucideIcon,
	Shuffle,
} from "lucide-react";

import { SIZE_PRESETS } from "@/types";
import type {
	ProjectCreateKind,
	ProjectItemAddRequest,
} from "../../editor/api/types";

// What a person can choose to create. The first three are the kinds the service knows; the last is the project
// that is not decided yet, which is created with no kind (the service reads it as undecided).
export const CHOICES = ["text", "video", "image", "undecided"] as const;
export type Choice = (typeof CHOICES)[number];

export const CHOICE_ICON: Record<Choice, LucideIcon> = {
	text: FileText,
	video: Clapperboard,
	image: ImageIcon,
	undecided: Shuffle,
};

// The types of text a person can start with; the service keeps them as the text kind of the first text item.
export const TEXT_TYPES = ["post", "copy", "script", "caption"] as const;
export type TextType = (typeof TEXT_TYPES)[number];

export { SIZE_PRESETS };

export interface NewProjectChoice {
	choice: Choice;
	textType: TextType;
	format: string;
}

// What the briefing ends in: the kind sent with the project and the first item that follows it, if any. Only a
// text or a video adds one: a text starts its first text with the type chosen, a video its first timeline with the
// format chosen. An image, or a project not decided yet, creates nothing but the project: no timeline, no format.
export interface CreatePlan {
	kind?: ProjectCreateKind;
	item?: ProjectItemAddRequest;
}

export function createPlan(pick: NewProjectChoice, title: string): CreatePlan {
	switch (pick.choice) {
		case "text":
			return {
				kind: "text",
				item: { kind: "text", title, text: "", text_kind: pick.textType },
			};
		case "video":
			return {
				kind: "video",
				item: { kind: "timeline", title, preset: pick.format },
			};
		case "image":
			return { kind: "image" };
		case "undecided":
			return {};
	}
}

// The sentences of the briefing that change with the kind. A kind with no example of its own uses the neutral one.
const OWN_EXAMPLE = new Set(["description", "specific", "achievable"]);

export function exampleKey(kind: Choice | undefined, field: string): string {
	return kind !== undefined && kind !== "undecided" && OWN_EXAMPLE.has(field)
		? `ui.projects.example_kind.${kind}.${field}`
		: `ui.projects.example.${field}`;
}
