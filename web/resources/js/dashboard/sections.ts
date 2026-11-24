import {
	AudioLines,
	Brain,
	Clapperboard,
	Film,
	Folder,
	ImagePlus,
	Images,
	KeyRound,
	type LucideIcon,
	MessageSquareText,
	Palette,
	PenLine,
	Plug,
	Send,
	ShieldCheck,
	Sparkles,
	Webhook,
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
	"webhooks",
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
	webhooks: Webhook,
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

export type NavGroupId = "create" | "assets" | "agent" | "publish" | "connect";

export interface NavItem {
	// the test id suffix and the key of the label
	id: string;
	href: string;
	icon: LucideIcon;
	// the key of the section when the item is one of SECTION_KEYS
	section?: SectionKey;
	// the translation key of the label
	label: string;
	// drawn only in network mode, the one that has users and keys
	networkOnly?: boolean;
	// the item carries the count of the requests that wait for a person
	counter?: "approvals";
}

export interface NavGroup {
	id: NavGroupId;
	label: string;
	items: readonly NavItem[];
}

function sectionItem(key: SectionKey): NavItem {
	return {
		id: key,
		href: `/${key}`,
		icon: ICONS[key],
		section: key,
		label: `ui.nav.section.${key}`,
	};
}

// The sidebar groups (SDD-012 section 23.4): a list over the keys of SECTION_KEYS plus the links to routes
// that already exist, so the keys, the Go route test and the home keep their meaning. A route that no
// epic has added yet (Record, Local models) is not here.
export const NAV_GROUPS: readonly NavGroup[] = [
	{
		id: "create",
		label: "ui.nav.group.create",
		items: [
			sectionItem("projects"),
			sectionItem("write"),
			sectionItem("edit-video"),
			sectionItem("edit-image"),
		],
	},
	{
		id: "assets",
		label: "ui.nav.group.assets",
		items: [
			sectionItem("video-assets"),
			sectionItem("visual-assets"),
			sectionItem("sound-assets"),
			{
				id: "brand-kit",
				href: "/settings/brand-kit",
				icon: Palette,
				label: "ui.nav.brand_kit",
			},
		],
	},
	{
		id: "agent",
		label: "ui.nav.group.agent",
		items: [
			sectionItem("skills"),
			sectionItem("prompts"),
			sectionItem("memory"),
		],
	},
	{
		id: "publish",
		label: "ui.nav.group.publish",
		items: [
			{
				id: "approvals",
				href: "/approvals",
				icon: ShieldCheck,
				label: "ui.nav.approvals",
				counter: "approvals",
			},
			sectionItem("publish"),
		],
	},
	{
		id: "connect",
		label: "ui.nav.group.connect",
		items: [
			sectionItem("connections"),
			sectionItem("webhooks"),
			{
				id: "keys",
				href: "/settings/keys",
				icon: KeyRound,
				label: "ui.keys.nav",
				networkOnly: true,
			},
		],
	},
];

// activeNavItem is the id of the item whose path holds the url, the longest path first. The home, and
// Settings itself, belong to no item.
export function activeNavItem(url: string): string | null {
	const path = pathOf(url);
	let best: NavItem | null = null;
	for (const group of NAV_GROUPS) {
		for (const item of group.items) {
			if (path !== item.href && !path.startsWith(`${item.href}/`)) continue;
			if (best === null || item.href.length > best.href.length) best = item;
		}
	}
	return best?.id ?? null;
}
