/**
 * The copy of the landing page (epic 88), English only. Every claim about the product carries the epics that deliver
 * it, and `content.test.ts` checks them against the status column of docs/plans/00-roadmap.md: a claim outside
 * `roadmap` needs every one of its epics DONE, and a feature that is not released lives only in `roadmap`. Commands
 * shown on the page come from `drive` in lib/landing-pages.ts, copied from the build. Never add a testimonial, a
 * customer logo, a number nobody measured or an award (ADR-0045, epic 88).
 */

export type Claim = {
	/** What the page says. */
	text: string;
	/** The epics of docs/plans/ that deliver it; every one must be DONE in the roadmap. */
	epics: number[];
};

export type Step = {
	id: string;
	title: string;
	text: string;
	/** Key of `shots` in lib/landing-pages.ts, or a slot of a screen that is not merged yet. */
	screen: { kind: "shot"; shot: "writing" | "videoEditor" | "imageEditor" | "approvals" | "calendar" } | { kind: "slot"; slot: ScreenSlotId };
	claims: Claim[];
};

/**
 * The screens of epic 79 (home, projects, briefing). Each has the captures of the released UI (`src` is the light one,
 * `srcDark` the dark one, both under public/ without the size suffix and the extension); a screen with no capture
 * would draw a named slot instead.
 */
export type ScreenSlotId = "home" | "projects" | "briefing";

export const screenSlots: Record<ScreenSlotId, { label: string; alt: string; src?: string; srcDark?: string }> = {
	home: {
		label: "The Stuarty home",
		alt: "The Stuarty home: the project to continue with its steps, what waits for you and your projects, each with its kind.",
		src: "/landing/dashboard-light",
		srcDark: "/landing/dashboard",
	},
	projects: {
		label: "The projects page",
		alt: "The projects page: a text, a video, an image and a project not decided yet, each as a card with its kind and when it was updated.",
		src: "/landing/projects-light",
		srcDark: "/landing/projects",
	},
	briefing: {
		label: "The guided briefing",
		alt: "The briefing of a new project: its name, description and goal, answered before the project is created.",
		src: "/landing/project-brief-light",
		srcDark: "/landing/project-brief",
	},
};

export const hero = {
	eyebrow: "Open source studio for creators",
	title: "Create, edit, manage and publish your content in one place.",
	lead: "Stuarty is a self-hosted studio for video, images and text, on the web and on your desktop. Your AI agent works beside you, and nothing is published until you say yes.",
	facts: ["Open source (MIT)", "Self-hosted", "No telemetry", "Agents are first-class users"],
};

export const path = {
	eyebrow: "One path",
	title: "From the idea to the post.",
	lead: "Every project follows the steps from its briefing to its publication, and shows which one is done and which is next.",
	steps: [
		{
			id: "project",
			title: "Start a project",
			text: "Name it and pick what it is: a text, a video, an image, or decide later. A project keeps its brief, its media, its texts and its exports together.",
			screen: { kind: "slot", slot: "projects" },
			claims: [
				{ text: "A project is the container of one production.", epics: [6, 56] },
				{ text: "A project can be only a text, a video, an image or undecided.", epics: [78] },
			],
		},
		{
			id: "brief",
			title: "Answer the brief",
			text: "Describe it and set its goal and due date before it is created. Then the guided briefing asks who it is for, where it goes, how long and in which tone. Your answers become the brief your agent reads.",
			screen: { kind: "slot", slot: "briefing" },
			claims: [
				{ text: "A new project asks for its description, its goal and its due date.", epics: [79] },
				{ text: "A guided briefing turns answers into the brief of the production.", epics: [16] },
			],
		},
		{
			id: "write",
			title: "Write the script",
			text: "Scripts, copy, captions and blog posts, with revisions you can go back to and an estimate of how long a script takes read aloud.",
			screen: { kind: "shot", shot: "writing" },
			claims: [{ text: "Writing with revisions and a read-aloud estimate.", epics: [17] }],
		},
		{
			id: "edit",
			title: "Cut and caption",
			text: "Trim, split, transitions, colour and keyframes, with captions that follow the voice. Cut by editing the transcript, remove filler words, style the captions, blur what must stay private and clean the voice. Export a checked MP4.",
			screen: { kind: "shot", shot: "videoEditor" },
			claims: [
				{ text: "The video editor, with captions and export.", epics: [37, 35] },
				{ text: "Edit by text, filler words, caption styles, sensitive-data blur and studio voice.", epics: [69, 71] },
			],
		},
		{
			id: "thumbnail",
			title: "Make the thumbnail",
			text: "Layers, text, masks and templates in the image editor, with the same undo and history as the video editor.",
			screen: { kind: "shot", shot: "imageEditor" },
			claims: [{ text: "The image editor, with layers, text, masks and templates.", epics: [28, 29] }],
		},
		{
			id: "publish",
			title: "Approve and publish",
			text: "Plan each post on a calendar, for your own accounts. It goes out only after you approve it.",
			screen: { kind: "shot", shot: "calendar" },
			claims: [{ text: "A publishing calendar where every post waits for approval.", epics: [20, 5] }],
		},
	] satisfies Step[],
};

export const transparency = {
	eyebrow: "Agent transparency",
	title: "See what the agent does, and decide.",
	lead: "An agent is a user like you, not a black box. What it touches is on screen in plain words, and what costs money or goes public waits for a person.",
	points: [
		{ title: "Its work is signed", text: "Every edit lands in the history with who made it, and you can undo it.", epics: [36, 14] },
		{ title: "You see its skill and its tool", text: "The scene bar shows the skill it loaded, whether that skill is active for the task, the tool it is using and what that tool does, in plain words.", epics: [75, 76] },
		{ title: "A person decides", text: "A publication or a paid generation waits in one queue until you approve it. An agent cannot approve its own request or raise a ceiling.", epics: [5, 20] },
		{ title: "You bring your own agent", text: "Claude Code, Codex, OpenCode or Gemini CLI, signed in with your own account, work in the chat panel beside the project.", epics: [14] },
	] satisfies { title: string; text: string; epics: number[] }[],
};

export const textProject = {
	eyebrow: "Not only video",
	title: "A text is a complete project.",
	lead: "A blog post, a newsletter or a script needs no footage and no format. Its steps are its own: brief, draft, review, then copy or export, or a blog when you choose.",
	points: ["Count words and reading time as you write.", "Every save is a revision you can compare or restore.", "Add a video or an image later without starting over."],
	claims: [
		{ text: "A project can be only a text, with the steps of a text.", epics: [78] },
		{ text: "Revisions and reading time of a text.", epics: [17] },
		{ text: "Blog posts with their destinations.", epics: [23] },
	] satisfies Claim[],
};

export const areas = {
	eyebrow: "Areas",
	title: "Everything a creator keeps in hand.",
	items: [
		{ id: "projects", title: "Projects", text: "Productions with their brief, media, texts and exports.", epics: [6, 56, 78] },
		{ id: "video", title: "Video", text: "A timeline editor with captions, editing by text, transitions, audio tools and export presets.", epics: [37, 34, 35, 69] },
		{ id: "image", title: "Images", text: "An editor for thumbnails and posts, with layers, filters and masks.", epics: [28, 29] },
		{ id: "write", title: "Writing", text: "Scripts, copy, captions and blog posts with revisions.", epics: [17, 23] },
		{ id: "assets", title: "Assets", text: "One library with where each file came from, its rights and where it is used.", epics: [8, 81] },
		{ id: "publish", title: "Publishing", text: "A calendar and an approval queue for your own social accounts and blog.", epics: [18, 19, 20, 22, 23] },
		{ id: "agents", title: "Skills, prompts and memory", text: "Procedures you review, texts with variables and facts your agents read.", epics: [15, 16] },
		{ id: "connections", title: "Connections", text: "The AI providers and accounts Stuarty may reach, each with your own key.", epics: [4, 9, 12, 84] },
	],
};

export const agents = {
	eyebrow: "Agents are first-class users",
	title: "Four doors to one command core.",
	lead: "Everything you do in the editor, an agent can do through MCP, the CLI or the HTTP API. The doors are thin adapters over one core, and a parity test keeps them equal.",
	epics: [13, 14],
};

export const privacy = {
	eyebrow: "Yours",
	title: "On your machine. On your terms.",
	points: [
		{ title: "Open source", text: "MIT licensed. Read it, build it, change it.", epics: [] as number[] },
		{ title: "Self-hosted", text: "Web and desktop, or your own server. Files and tokens stay with you.", epics: [21, 24, 25] },
		{ title: "No telemetry", text: "With nothing connected, Stuarty opens no outbound connection.", epics: [4] },
		{ title: "A local vault", text: "Your keys and tokens live in an encrypted vault, and no tool ever returns one.", epics: [4, 57] },
		{ title: "Your own access", text: "Your AI key, Sign in with ChatGPT, OpenRouter or your local agent. Your own developer apps for the social networks.", epics: [9, 12, 18] },
		{ title: "Publishing waits for you", text: "An agent can prepare a post. Only a person, or a rule you wrote, can send it.", epics: [5, 20] },
	],
};

export const downloads = {
	eyebrow: "Download",
	title: "Try it on your machine.",
	lead: "Stuarty is in alpha: expect rough edges. Free and open source, for Windows, macOS, Linux and your own server.",
};

export type RoadmapItem = {
	id: string;
	title: string;
	text: string;
	/**
	 * `in-progress`: at least one epic is IN PROGRESS; `planned`: no epic is DONE. An epic DONE on the integration branch
	 * ships with this page (the site is built from `main` when a release is cut), so it never stays on the roadmap.
	 */
	state: "in-progress" | "planned";
	epics: number[];
};

/** The only place for what is not released. A feature leaves this list in the change that releases it. */
export const roadmap = {
	eyebrow: "On the roadmap",
	title: "Not released yet.",
	lead: "These are planned or in progress. Something in progress may have a first part in the product you download today, but it is not finished.",
	stateLabel: { "in-progress": "In progress", planned: "Planned" } as Record<RoadmapItem["state"], string>,
	items: [
		{
			id: "recording",
			title: "Screen recording",
			text: "A first screen capture on Windows is in the product while its performance is measured. Audio, webcam and pause come next, then recording on the web, macOS and Linux, with the take edited in the same project.",
			state: "in-progress",
			epics: [61, 62, 63, 67, 68],
		},
		{
			id: "recording-edit",
			title: "Automatic zoom, cursor and layouts",
			text: "Follow the cursor with an automatic zoom, highlight clicks, and set layouts, backgrounds and a teleprompter for a recording.",
			state: "planned",
			epics: [65, 66],
		},
		{
			id: "local-ai",
			title: "Optional local models",
			text: "Optional local models for background removal and depth, running on your machine.",
			state: "planned",
			epics: [72],
		},
		{
			id: "agents-recording",
			title: "Agents in the recording flow",
			text: "Let an agent start, review and cut a recording through the same doors it uses today.",
			state: "planned",
			epics: [64],
		},
		{
			id: "metrics",
			title: "Metrics of published posts",
			text: "How each published post performs, read from your own accounts.",
			state: "planned",
			epics: [45],
		},
	] satisfies RoadmapItem[],
};

/** Words that name a roadmap-only feature: they must not appear in the copy outside `roadmap`. */
export const roadmapOnlyTerms = ["recording", "record your", "screen capture", "webcam", "teleprompter", "local model", "face-aware", "auto zoom", "automatic zoom", "metrics"];

export const credits = "The pictures, clips and sounds in the screenshots were made for the demo. No real person, brand or number is shown.";

/**
 * The screenshots are real captures of the UI that ships with this page, from a demo workspace, taken on this date. They
 * name no version: the build reported v0.52.0, but the home, projects and briefing screens (epic 79) came after it.
 */
export const capturedNote = "Screenshots are captures of the Stuarty UI taken on 24 November 2026, from a demo workspace.";
