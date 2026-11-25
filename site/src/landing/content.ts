/**
 * The copy of the landing page (epic 95), English only, word for word from the final design of the landing. Every claim
 * about the product carries the epics that deliver it, and `content.test.ts` checks them against the status column of
 * docs/plans/00-roadmap.md: a claim outside `roadmap` needs every one of its epics DONE, and a feature that is not
 * released lives only in `roadmap`. The commands and the demo of the agent tab come from `drive` and `homeDemo` in
 * lib/landing-pages.ts, copied from the build. Never add a testimonial, a customer logo, a number nobody measured or an
 * award (ADR-0045, epics 88 and 95).
 */

/** A real capture of the released UI: a dark and a light picture, `src` names the dark one. */
export type Screen = {
	/** Base path under public/, without the size suffix, the theme suffix and the extension. */
	src: string;
	alt: string;
	/** The size of the capture, from its 1600 px file. */
	width: number;
	height: number;
};

export type Claim = {
	/** What the page says. */
	text: string;
	/** The epics of docs/plans/ that deliver it; every one must be DONE in the roadmap. */
	epics: number[];
};

/** The icons of the page, by the name of their lucide icon (components/landing/Icon.astro). */
export type IconName =
	| "folder-open"
	| "compass"
	| "pen-line"
	| "images"
	| "clapperboard"
	| "image"
	| "bot"
	| "brain"
	| "plug"
	| "shield-check"
	| "wand-sparkles"
	| "terminal"
	| "workflow"
	| "hard-drive"
	| "video"
	| "mouse-pointer-click"
	| "scan-face"
	| "cpu"
	| "file-clock"
	| "layout-template";

/** The product images of the page, in both themes. Every one is a capture of the released UI (CREDITS.txt). */
export const screens = {
	home: {
		src: "/landing/dashboard",
		alt: "The Stuarty home: the project in progress with its path from briefing to publication, what waits for your yes and your projects.",
		width: 1600,
		height: 1000,
	},
	homeMobile: {
		src: "/landing/home-mobile",
		alt: "The Stuarty home on a phone: the project in progress, what waits for your yes and the scene bar at the bottom.",
		width: 1600,
		height: 3464,
	},
	briefing: {
		src: "/landing/project-brief",
		alt: "The briefing of a new project: its name, description and goal, answered before the project is created.",
		width: 1600,
		height: 1000,
	},
	editor: {
		src: "/landing/video-editor",
		alt: "The Stuarty video editor on a demo project: a vertical clip with a caption, the media bin on the left, the inspector on the right and a timeline with caption, footage, voice and music tracks.",
		width: 1600,
		height: 1000,
	},
	approvals: {
		src: "/landing/approvals",
		alt: "Approvals: what waits for your yes, a voice-over with its estimated cost and a blog post with its text, each to approve or reject.",
		width: 1600,
		height: 1000,
	},
	calendar: {
		src: "/landing/publish-calendar",
		alt: "The Publish section: the month view of the publishing calendar, with a note that every post waits for your approval before it is sent.",
		width: 1600,
		height: 1000,
	},
	trail: {
		src: "/landing/project-trail",
		alt: "The path of a video project, from briefing to publication: each step says who does it, with which tool and what it costs.",
		width: 1600,
		height: 1000,
	},
	projects: {
		src: "/landing/projects",
		alt: "The projects page: a video, an image and a text project as cards, each with its kind, and the templates to start from.",
		width: 1600,
		height: 1000,
	},
	writing: {
		src: "/landing/writing",
		alt: "The Write section: the script of the video project open, with its words, its reading time, its revision and the path of the project.",
		width: 1600,
		height: 1000,
	},
	videoAssets: {
		src: "/landing/video-assets",
		alt: "Video Assets: the footage of the workspace, with where each file came from, its tags and when it was added.",
		width: 1600,
		height: 1000,
	},
	imageEditor: {
		src: "/landing/image-editor",
		alt: "The image editor: a thumbnail with a picture, a dark band and its title, the layers panel, the inspector of the selected title and the history.",
		width: 1600,
		height: 1000,
	},
	agentChat: {
		src: "/landing/agent-chat",
		alt: "The agent chat beside a project: the agents Stuarty knows, Claude Code, Codex, OpenCode and Gemini CLI, and how to connect the one you use.",
		width: 1600,
		height: 1000,
	},
	skills: {
		src: "/landing/skills",
		alt: "Skills: the curated skills and one of your own, which no agent is offered until you have read it.",
		width: 1600,
		height: 1000,
	},
	connections: {
		src: "/landing/connections",
		alt: "Connections: the encrypted vault and the AI providers, each connected with your own key.",
		width: 1600,
		height: 1000,
	},
	textTrail: {
		src: "/landing/text-trail",
		alt: "The path of a text project: briefing, writing, review, approval and publication, with no timeline and no media.",
		width: 1600,
		height: 1000,
	},
} satisfies Record<string, Screen>;

/**
 * The hero, as the author changed it after the review of epic 95 (2026-11-24): no eyebrow, a centred title with no final
 * full stop whose first words carry the colours of the accent palette, no alpha note, and the download and setup cards in
 * place of the two buttons.
 */
export const hero = {
	title: "Create, edit, manage and publish your audiovisual and text content in one place",
	/** The start of `title` drawn in the colours of the accent palette: the reading focus. */
	titleFocus: "Create, edit, manage and publish",
	lead: "Plan, describe, cut, caption, thumbnail, post texts, images and videos from one studio on your own machine. Your AI agent works for and beside you, all integrated.",
	/** The phone layout of the design shortens the lead and the note under the buttons. */
	leadMobile: "Write, cut, caption and post from one studio on your own machine. Your AI agent works for and beside you.",
	/** The name of the download and setup cards under the lead, which replaced the two buttons of the hero. */
	downloads: "Download Stuarty or set it up",
	note: "Free and open source. No subscriptions.",
	claims: [
		{ text: "An AI agent works for and beside you, through the same commands.", epics: [13, 14] },
		{ text: "Texts, images and videos from one studio.", epics: [17, 28, 37, 78] },
	] satisfies Claim[],
};

export type Step = {
	id: string;
	title: string;
	text: string;
	screen: keyof typeof screens;
	claims: Claim[];
};

export const path = {
	eyebrow: "One path",
	title: "From the first idea to the published post.",
	lead: "Stuarty always shows where you are and what comes next, so you can learn it by using it.",
	/**
	 * The path of a project, one main screen per step, as the author asked after the review of epic 95 (2026-11-24),
	 * in the order of the "How it works" section of the README.
	 */
	steps: [
		{
			id: "home",
			title: "Open the home",
			text: "The home shows the project to continue, the step it is on and what waits for your yes.",
			screen: "home",
			claims: [{ text: "The home shows the project to continue, the step it is on and what waits for your yes.", epics: [7, 79] }],
		},
		{
			id: "project",
			title: "Start a project",
			text: "Name a project and pick what it is: a text, a video, an image, or decide later. It keeps its brief, its media, its texts and its exports together.",
			screen: "projects",
			claims: [{ text: "Name a project and pick what it is: a text, a video, an image, or decide later. It keeps its brief, its media, its texts and its exports together.", epics: [6, 56, 78] }],
		},
		{
			id: "brief",
			title: "Answer the brief",
			text: "Answer a few plain questions about the audience, the goal and the tone. The brief becomes the context your agent reads before it suggests anything.",
			screen: "briefing",
			claims: [{ text: "Answer a few plain questions about the audience, the goal and the tone. The brief becomes the context your agent reads before it suggests anything.", epics: [16, 79] }],
		},
		{
			id: "write",
			title: "Write the script",
			text: "Scripts, copy, captions and blog posts, where every save is a revision you can go back to. A text is a complete project on its own.",
			screen: "writing",
			claims: [{ text: "Scripts, copy, captions and blog posts, where every save is a revision you can go back to. A text is a complete project on its own.", epics: [17, 80] }],
		},
		{
			id: "media",
			title: "Bring in your media",
			text: "Import footage, music, voice-overs and pictures once and reuse them in any project, with where each one came from.",
			screen: "videoAssets",
			claims: [{ text: "Import footage, music, voice-overs and pictures once and reuse them in any project, with where each one came from.", epics: [8, 58, 81] }],
		},
		{
			id: "edit",
			title: "Cut and caption",
			text: "Cut, caption and mix on a timeline, by hand or with your agent. The screen shows what the server confirmed.",
			screen: "editor",
			claims: [{ text: "Cut, caption and mix on a timeline, by hand or with your agent. The screen shows what the server confirmed.", epics: [37, 71, 85] }],
		},
		{
			id: "thumbnail",
			title: "Make the thumbnail",
			text: "Layers, text, masks and templates for thumbnails, covers and posts, with the same undo and history as the video editor.",
			screen: "imageEditor",
			claims: [{ text: "Layers, text, masks and templates for thumbnails, covers and posts, with the same undo and history as the video editor.", epics: [28, 29, 50, 86] }],
		},
		{
			id: "agent",
			title: "Work with your agent",
			text: "Claude Code, Codex, OpenCode or Gemini CLI work beside the project with your own sign-in. You see the skill it loaded and the tool it is using.",
			screen: "agentChat",
			claims: [{ text: "Claude Code, Codex, OpenCode or Gemini CLI work beside the project with your own sign-in. You see the skill it loaded and the tool it is using.", epics: [14, 76, 83] }],
		},
		{
			id: "teach",
			title: "Teach it how you work",
			text: "Skills are procedures you review, prompts are texts with variables, and memory holds the facts your agents read once you keep them.",
			screen: "skills",
			claims: [{ text: "Skills are procedures you review, prompts are texts with variables, and memory holds the facts your agents read once you keep them.", epics: [15, 16, 83] }],
		},
		{
			id: "approve",
			title: "Approve",
			text: "Agents prepare. Only you say yes. Every publication and paid generation waits in one queue, with its cost in sight.",
			screen: "approvals",
			claims: [{ text: "Agents prepare. Only you say yes. Every publication and paid generation waits in one queue, with its cost in sight.", epics: [5, 20, 77, 82] }],
		},
		{
			id: "publish",
			title: "Publish",
			text: "Plan the month, the week or a list. A post goes out only after your approval.",
			screen: "calendar",
			claims: [{ text: "Plan the month, the week or a list. A post goes out only after your approval.", epics: [5, 20, 82] }],
		},
		{
			id: "connect",
			title: "Connect only what you choose",
			text: "With nothing connected, Stuarty opens no outbound connection. Each AI provider or account you add uses your own key, kept in a local encrypted vault.",
			screen: "connections",
			claims: [{ text: "With nothing connected, Stuarty opens no outbound connection. Each AI provider or account you add uses your own key, kept in a local encrypted vault.", epics: [4, 9, 12, 84] }],
		},
	] satisfies Step[],
};

export const transparency = {
	eyebrow: "Nothing hidden",
	title: "You always know what the agent is doing.",
	lead: "The skill in use, whether it is loaded, the tool at work in plain words, and every stage up to the published post.",
	points: [
		{ icon: "wand-sparkles", title: "The skill, by name", text: "Loaded, available or none. Always in words, never only a color.", epics: [75, 76] },
		{ icon: "terminal", title: "The tool, in plain words", text: "What it is about to do, with the technical name right beside it.", epics: [75, 76] },
		{ icon: "workflow", title: "The whole path", text: "Each stage to publication: who does it, what it costs, what comes next.", epics: [78, 79] },
	] satisfies { icon: IconName; title: string; text: string; epics: number[] }[],
	screen: "trail" as keyof typeof screens,
};

export const textProject = {
	eyebrow: "Not only video",
	title: "A text is a complete project.",
	lead: "A blog post, a copy or a script needs no timeline, no format and no media. Write it, review it, approve it and send it to your blog.",
	screen: "textTrail" as keyof typeof screens,
	claims: [
		{ text: "A project can be only a text, with the steps of a text.", epics: [78, 80] },
		{ text: "A blog post is sent to your blog after your approval.", epics: [23, 82] },
	] satisfies Claim[],
};

export const areas = {
	eyebrow: "One studio",
	title: "Every part of the work, under one roof.",
	items: [
		{ id: "projects", icon: "folder-open", title: "Projects", text: "A text, a video or an image, from the brief to the post. A format only when you add a timeline.", epics: [6, 56, 78] },
		{ id: "briefing", icon: "compass", title: "Briefing", text: "Guided questions on audience, goal and tone. Your agent reads the saved brief.", epics: [16, 79] },
		{ id: "writing", icon: "pen-line", title: "Writing", text: "Scripts, captions and blog posts with revisions and read-aloud timing.", epics: [17, 23] },
		{ id: "assets", icon: "images", title: "Assets", text: "Footage, music, voice-overs and images, tagged by origin and reusable.", epics: [8, 81] },
		{ id: "video", icon: "clapperboard", title: "Video editor", text: "Trims, transitions, word-timed captions, silence removal. MP4, GIF, WAV, AAC.", epics: [37, 34, 35, 69] },
		{ id: "image", icon: "image", title: "Image editor", text: "Layers, text, masks, presets and templates for thumbnails and posts.", epics: [28, 29] },
		{ id: "agents", icon: "bot", title: "Agents", text: "Claude Code, Codex, OpenCode and Gemini CLI. You approve each tool request.", epics: [14, 83] },
		{ id: "library", icon: "brain", title: "Skills, prompts, memory", text: "Reviewed procedures, reusable prompts and notes you approve before they stay.", epics: [15, 16, 83] },
		{ id: "recording", icon: "video", title: "Screen recording", text: "On Windows, with sound, webcam, pause and pointer events.", epics: [62] },
		{ id: "edit-by-text", icon: "wand-sparkles", title: "Edit by text", text: "Cut a video by editing its transcript, drop filler words, style captions, blur private details and clean the voice.", epics: [69, 71] },
		{ id: "connections", icon: "plug", title: "Connections", text: "Your own AI keys and social accounts, in an encrypted local vault.", epics: [4, 9, 12, 84] },
		{ id: "publishing", icon: "shield-check", title: "Approvals and publishing", text: "A queue, cost ceilings and a calendar. Nothing is sent without your yes.", epics: [5, 20, 82] },
	] satisfies { id: string; icon: IconName; title: string; text: string; epics: number[] }[],
};

export const agents = {
	eyebrow: "Agent-first",
	title: "Agents drive it too.",
	lead: "The editor, the CLI, the HTTP API and MCP are thin doors to one command core, tested for parity.",
	epics: [13, 14],
	permissionTitle: "Every tool call asks first",
};

export const privacy = {
	title: "On your machine. On your terms.",
	subtitle: "No subscriptions. No pay once. Open Source.",
	points: [
		{ icon: "hard-drive", title: "Your files stay here", text: "Projects, media and credentials live on your machine or your own server.", epics: [21, 24, 25, 57] },
		{ icon: "plug", title: "Nothing leaves by default", text: "With no connection set up, Stuarty opens no outbound connection. No telemetry.", epics: [4] },
		{ icon: "shield-check", title: "You hold the last word", text: "An agent can prepare. It can never approve, publish or raise a limit.", epics: [5, 20] },
	] satisfies { icon: IconName; title: string; text: string; epics: number[] }[],
};

export const downloads = {
	eyebrow: "Free and open source",
	title: "Make the next one in one place.",
	lead: "Runs on Windows, macOS, Linux and your own server.",
	primary: "Get started",
	secondary: "Read the docs",
	label: "Downloads and setup guides",
};

export type RoadmapItem = {
	id: string;
	icon: IconName;
	title: string;
	text: string;
	/**
	 * `in-progress`: at least one epic is IN PROGRESS; `planned`: no epic is DONE. An epic DONE on the integration branch
	 * ships with this page (the site is built from `main` when a release is cut), so it never stays on the roadmap.
	 */
	state: "in-progress" | "planned";
	epics: number[];
};

/**
 * The only place for what is not released. A feature leaves this list in the change that releases it. The design shows
 * six cards here; three of them (edit by text, vertical cuts, caption styles with blur and studio voice) are epics 69, 70
 * and 71, which are DONE and ship with this page, so they are not here: the test of the roadmap refuses a finished epic.
 * Their places hold other planned work. Screen recording on Windows (epic 62) left this list after the review of epic 95
 * and is a card of `areas`; recording on the web, macOS and Linux stays here.
 */
export const roadmap = {
	badge: "Also in Stuarty",
	title: "More of the studio, in the same place.",
	lead: "Record on any system, polish the take, let your agent help, and see how each post performs.",
	stateLabel: { "in-progress": "In progress", planned: "Planned" } as Record<RoadmapItem["state"], string>,
	items: [
		{
			id: "recording-more",
			icon: "video",
			title: "Recording on every system",
			text: "Record on the web, macOS and Linux, just as on Windows.",
			state: "planned",
			epics: [63, 67, 68],
		},
		{
			id: "recording-finish",
			icon: "mouse-pointer-click",
			title: "Recording finish",
			text: "The zoom follows your clicks, the cursor moves smoothly and every click is highlighted.",
			state: "planned",
			epics: [65],
		},
		{
			id: "agents-recording",
			icon: "bot",
			title: "Agents in the recording flow",
			text: "Your agent prepares, marks takes, pauses and stops. Only a person starts a capture.",
			state: "planned",
			epics: [64],
		},
		{
			id: "layouts",
			icon: "layout-template",
			title: "Layouts and teleprompter",
			text: "Choose a layout and a background for each recording, and read your script from the teleprompter.",
			state: "planned",
			epics: [66],
		},
		{
			id: "metrics",
			icon: "file-clock",
			title: "Post metrics",
			text: "See how each published post performs, read from your own accounts.",
			state: "planned",
			epics: [45],
		},
		{
			id: "local-ai",
			icon: "cpu",
			title: "Optional local models",
			text: "Run small models on your machine, downloaded on request, with size and licence shown first.",
			state: "planned",
			epics: [72],
		},
	] satisfies RoadmapItem[],
};

/** Words that name a roadmap-only feature: they must not appear in the copy outside `roadmap`. */
export const roadmapOnlyTerms = [
	// Recording ships on Windows (epic 62); on the web, macOS and Linux, its finish and its agent tools it does not.
	"recording on the web", "web recording", "recording on macos", "recording on linux", "record your", "screen capture",
	"zoom on clicks", "smooth cursor", "click highlight", "mark takes", "teleprompter", "local model", "face-aware",
	"auto zoom", "automatic zoom", "metrics",
];

export const credits = "The pictures, clips and sounds in the screenshots were made for the demo. No real person, brand or number is shown.";

/**
 * The screenshots are real captures of the UI that ships with this page, from a demo workspace, taken on this date. They
 * name no version: the sidebar of the capture shows the version of the build that took it.
 */
export const capturedNote = "Screenshots are captures of the Stuarty UI taken on 24 November 2026, from a demo workspace.";

/** The labels of the zoom of a screenshot (components/landing/HomeShot.astro and ZoomDialog.astro). */
export const zoom = {
	enlarge: "Enlarge: ",
	close: "Close",
	dialog: "Enlarged screenshot",
};
