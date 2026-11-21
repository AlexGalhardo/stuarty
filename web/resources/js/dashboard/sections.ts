import {
	AudioLines,
	Brain,
	Clapperboard,
	Film,
	Folder,
	ImagePlus,
	Images,
	type LucideIcon,
	MessageSquareText,
	PenLine,
	Plug,
	Send,
	Sparkles,
} from "lucide-react";

// The sections of the dashboard sidebar, in the order of RF-311 (PRD-004) and
// author decision D-075. The paths are the ones web/routes/dashboard.go serves;
// a Go test compares the two lists (SDD-012 section 23.1).
export const SECTION_KEYS = [
	"projects",
	"edit-video",
	"edit-image",
	"write",
	"visual-assets",
	"video-assets",
	"sound-assets",
	"publish",
	"skills",
	"prompts",
	"memory",
	"connections",
] as const;

export type SectionKey = (typeof SECTION_KEYS)[number];

export interface Section {
	key: SectionKey;
	path: string;
	icon: LucideIcon;
}

const ICONS: Record<SectionKey, LucideIcon> = {
	projects: Folder,
	"edit-video": Clapperboard,
	"edit-image": ImagePlus,
	write: PenLine,
	"visual-assets": Images,
	"video-assets": Film,
	"sound-assets": AudioLines,
	publish: Send,
	skills: Sparkles,
	prompts: MessageSquareText,
	memory: Brain,
	connections: Plug,
};

export const SECTIONS: readonly Section[] = SECTION_KEYS.map((key) => ({
	key,
	path: `/${key}`,
	icon: ICONS[key],
}));

export function isSectionKey(value: unknown): value is SectionKey {
	return SECTION_KEYS.some((key) => key === value);
}

// pathOf drops the query and the fragment of an Inertia page url.
export function pathOf(url: string): string {
	return url.split(/[?#]/, 1)[0] || "/";
}

// activeSection is the section whose path holds the url: the path itself or a
// page below it. Settings and the home belong to no section.
export function activeSection(url: string): SectionKey | null {
	const path = pathOf(url);
	const hit = SECTIONS.find(
		(s) => path === s.path || path.startsWith(`${s.path}/`),
	);
	return hit?.key ?? null;
}
