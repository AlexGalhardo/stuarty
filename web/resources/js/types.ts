// Shapes of the props the Go controllers send. Project data comes from
// service.Service (SDD-003 section 2.3) and is only drawn here (SDD-012 T1).

// ProjectSummary mirrors the Go type of the same name.
export interface ProjectSummary {
	id: string;
	name: string;
	revision: number;
	state: "active" | "trashed";
	width: number;
	height: number;
	fps: string;
	duration: { frame: number; seconds: number };
	tracks: number;
	items: number;
	assets: number;
	// The first timeline, the one the thumbnail and the editor show, and how many the production has.
	timeline_id: string;
	timelines: number;
	updated_at: string;
}

// The size presets of RF-002; the core owns what each one means.
export const SIZE_PRESETS = ["9:16", "1:1", "16:9", "4:5"] as const;
export type SizePreset = (typeof SIZE_PRESETS)[number];

export interface ProjectsIndexProps {
	projects: ProjectSummary[];
	[key: string]: unknown;
}

export interface User {
	id: string;
	email: string;
	role: string;
}

export interface SharedProps {
	appName: string;
	errors: Record<string, string>;
	// Present in network mode; `user` is null before the login.
	auth?: { user: User | null };
	// What the server says of the session: whether it is a person's (VV-319).
	session?: { privileged: boolean; mode: "local" | "network" };
	[key: string]: unknown;
}
