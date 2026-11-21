/**
 * Copy of the landing page (/) and of its two backup versions (/landing/classic, /landing/creative), English only.
 * The commands and calls of `drive` are copied from the build: `stuarty --help`, web/contract/testdata/surface.txt,
 * the agent docs in web/app/agentdocs/out/ and the generated MCP reference; the outputs are the real answers of a
 * demo workspace. Never write one from memory.
 */

export type Shot = {
	/** Base path under public/, without the size suffix and the extension. */
	src: string;
	alt: string;
	width: number;
	height: number;
};

export const shots = {
	videoEditor: {
		src: "/landing/video-editor",
		alt: "The Stuarty video editor: a vertical Big Buck Bunny clip with the caption \"Your agent edits with you\", the media bin on the left, the project inspector on the right, and a timeline with caption, video, voice and music tracks.",
		width: 1600,
		height: 1000,
	},
	videoEditorLight: {
		src: "/landing/video-editor-light",
		alt: "The Stuarty video editor in the light theme, with the same timeline of captions, video, voice and music.",
		width: 1600,
		height: 1000,
	},
	imageEditor: {
		src: "/landing/image-editor",
		alt: "The Stuarty image editor: a 1280 by 720 thumbnail of a mountain valley titled \"Make it. Ship it.\", the layers panel, the inspector, and a history where the last steps are signed local:claude.",
		width: 1600,
		height: 1000,
	},
	writing: {
		src: "/landing/writing",
		alt: "The Write section: a voice-over script in Markdown, with its revision and its estimated reading time.",
		width: 1600,
		height: 1000,
	},
	calendar: {
		src: "/landing/publish-calendar",
		alt: "The Publish section: the month view of the publishing calendar, with a note that every post waits for your approval before it is sent.",
		width: 1600,
		height: 1000,
	},
	agentChat: {
		src: "/landing/agent-chat",
		alt: "The agent chat panel beside a project: it hosts the agent you already use, Claude Code, Codex, OpenCode or Gemini CLI, with your own sign-in.",
		width: 1600,
		height: 1000,
	},
	dashboardLight: {
		src: "/landing/dashboard-light",
		alt: "The Stuarty home in the light theme: the sections a user can open, from projects and the editors to publishing and connections.",
		width: 1600,
		height: 1000,
	},
	dashboard: {
		src: "/landing/dashboard",
		alt: "The Stuarty home: projects, the video and image editors, writing, visual and video assets, publishing, skills, prompts, memory and connections.",
		width: 1600,
		height: 1000,
	},
	projects: {
		src: "/landing/projects",
		alt: "The Projects page: a form to create a project with its size (9:16, 1:1, 16:9 or 4:5) and the list of your projects.",
		width: 1600,
		height: 1000,
	},
	projectBrief: {
		src: "/landing/project-brief",
		alt: "The guided briefing of a project: who the video is for, its goal, the platform, the format, the length and the tone.",
		width: 1600,
		height: 1000,
	},
	visualAssets: {
		src: "/landing/visual-assets",
		alt: "Visual Assets: the images of the workspace with where each came from, filters by origin and tag.",
		width: 1600,
		height: 1000,
	},
	videoAssets: {
		src: "/landing/video-assets",
		alt: "Video Assets: the footage, music and voice-overs of the workspace, ready to reuse in any project.",
		width: 1600,
		height: 1000,
	},
	skills: {
		src: "/landing/skills",
		alt: "Skills: reviewed procedures an agent follows, such as captions for shorts, with their full text and lint.",
		width: 1600,
		height: 1000,
	},
	prompts: {
		src: "/landing/prompts",
		alt: "Prompts: reusable texts with variables, such as five hook lines for a short video.",
		width: 1600,
		height: 1000,
	},
	memory: {
		src: "/landing/memory",
		alt: "Memory: one fact per note that your agents read before they plan, kept only after a person approves it.",
		width: 1600,
		height: 1000,
	},
	connections: {
		src: "/landing/connections",
		alt: "Connections: the AI providers and social accounts Stuarty may reach, each connected with your own key.",
		width: 1600,
		height: 1000,
	},
	approvals: {
		src: "/landing/approvals",
		alt: "Approvals: what waits for your yes, the rules that approve without asking and the cost ceilings.",
		width: 1600,
		height: 1000,
	},
} satisfies Record<string, Shot>;

export const hero = {
	/** The title of the home page (the author's wording); the backup versions keep `title`. */
	homeTitle: "Create, edit, manage and publish your audiovisual and text content in one place",
	eyebrow: "Open source studio for creators",
	title: "Create, edit, manage and publish in one place.",
	lead: "Script, cut, caption, thumbnail and post from one studio on your own machine. Your AI agent works beside you, through the same commands you use.",
	/** The three phrases the hero title types, in the author's words; the first is the accessible name. */
	homeTitles: [
		[{ text: "Create, edit, manage and publish your audiovisual and text content " }, { text: "in one place", tone: "orange" }],
		[
			{ text: "Why use so many tools to create your content, when you can focus on " },
			{ text: "Stuarty", tone: "pink" },
			{ text: " and connect to other tools and services?" },
		],
		[
			{ text: "Open source, self-hosted studio content creation for the web and on the desktop " },
			{ text: "using agents", tone: "green" },
			{ text: " and your favorite AI providers" },
		],
	] as { text: string; tone?: "orange" | "pink" | "green" }[][],
	homeLead: "Plan, describe, cut, caption, thumbnail, post texts, images and videos from one studio on your own machine. Your AI agent works for and beside you, all integrated.",
	primary: { label: "Make your first short", href: "/docs/guide/first-short-video/" },
	secondary: { label: "Read the docs", href: "/docs/introduction/" },
};

export const problem = {
	eyebrow: "The problem",
	title: "Five tools for one short video.",
	lead: "The script lives in one app, the cut in another, the thumbnail in a third, the schedule in a fourth, and a fifth charges you to add AI on top. Your files and tokens end up in five clouds, and no agent can drive all of it.",
	tools: ["Script", "Video", "Thumbnail", "Schedule", "AI"],
	answer: "Stuarty is the one place for all of it.",
};

export type Feature = {
	id: string;
	verb: string;
	title: string;
	text: string;
	shot: Shot;
	/** The clip that replaces the placeholder later. */
	clip: string;
};

export const features: Feature[] = [
	{
		id: "create",
		verb: "Create",
		title: "Start from a brief, not a blank page",
		text: "A guided briefing turns your answers into the brief of the production. Write the script with revisions and a read-aloud estimate, alone or with a model you bring.",
		shot: shots.writing,
		clip: "Briefing to script, 8 s",
	},
	{
		id: "edit",
		verb: "Edit",
		title: "A real editor for video and images",
		text: "Trim, split, captions that follow the voice, transitions, colour and keyframes. Then make the thumbnail in the image editor, with layers, text and the same undo.",
		shot: shots.videoEditor,
		clip: "Captions and a cut, 10 s",
	},
	{
		id: "manage",
		verb: "Manage",
		title: "Projects as productions",
		text: "Each project keeps its brief, its media, its texts and its exports. A shared library holds your assets, prompts, skills and what your agents should remember.",
		shot: shots.dashboard,
		clip: "A production from brief to export, 8 s",
	},
	{
		id: "publish",
		verb: "Publish",
		title: "Publishing waits for a person",
		text: "Plan each post on a calendar for your own accounts, through your own developer apps. An agent can prepare a post; it goes out only after you approve it.",
		shot: shots.calendar,
		clip: "Schedule and approve a post, 8 s",
	},
];

export type DriveTab = {
	id: "agent" | "cli" | "http" | "mcp";
	label: string;
	title: string;
	text: string;
	/**
	 * A Claude Code session, line by line: `cmd` a shell command, `prompt` what the person asks, `tool` a tool call
	 * of Claude Code, `out` its result and `more` the next lines of that result, `note` a comment.
	 */
	lines: { kind: "cmd" | "prompt" | "tool" | "out" | "more" | "note"; text: string }[];
	/** What the copy button copies (the commands only). */
	copy: string;
	doc: { label: string; href: string };
};

// Every answer below is the real output of a demo workspace (2026-11-20): `stuarty project:show`, `lint`, `history`
// and `export`, the MCP tools `lint` and `history` and the HTTP routes of web/contract/testdata/surface.txt.
const curlEdit = `curl -s -X POST "http://127.0.0.1:7878/api/v1/projects/prj_6jzec0m2nn/edit" -H "Authorization: Bearer $STUARTY_KEY" -H "Content-Type: application/json" -d '{"base_revision": 5, "dry_run": true, "operations": [{"op": "caption.add", "args": {"start": "14.5s", "text": "Approve and publish", "duration": "3s"}}]}'`;

export const drive: DriveTab[] = [
	{
		id: "agent",
		label: "Agent",
		title: "Claude Code edits beside you",
		text: "Add Stuarty to Claude Code once. Ask in plain words: it reads the project, checks it and edits it, and every change lands on the timeline signed local:claude.",
		lines: [
			{ kind: "cmd", text: "claude mcp add stuarty -- stuarty mcp --actor claude" },
			{ kind: "cmd", text: "claude" },
			{ kind: "prompt", text: "Check Launch teaser, then add the caption \"Approve and publish\" at 14.5 s." },
			{ kind: "tool", text: 'stuarty - project_get (MCP)(project_id: "prj_6jzec0m2nn")' },
			{ kind: "out", text: 'project prj_6jzec0m2nn "Launch teaser" rev 5' },
			{ kind: "more", text: "1080x1920 30fps duration=22s/660f tracks=4 items=9 assets=3" },
			{ kind: "tool", text: 'stuarty - lint (MCP)(project_id: "prj_6jzec0m2nn")' },
			{ kind: "out", text: "lint r5 prj_6jzec0m2nn: errors=0 warnings=1 (showing 1 of 1)" },
			{ kind: "tool", text: 'stuarty - edit (MCP)(project_id: "prj_6jzec0m2nn", base_revision: 5, operations: [{"op": "caption.add", ...}])' },
			{ kind: "out", text: '{"revision": 6, "base_revision": 5, "applied": true, "actor": "local:claude", ...}' },
		],
		copy: "claude mcp add stuarty -- stuarty mcp --actor claude",
		doc: { label: "Connect an agent", href: "/docs/mcp/overview/" },
	},
	{
		id: "cli",
		label: "CLI",
		title: "Claude Code runs the stuarty CLI",
		text: "Every action of the editor is a command. Claude Code runs them in its shell, reads the plain-text answers and adds --json when it needs fields.",
		lines: [
			{ kind: "cmd", text: "claude" },
			{ kind: "prompt", text: "What changed in Launch teaser? Then export it." },
			{ kind: "tool", text: "Bash(stuarty history --project prj_6jzec0m2nn)" },
			{ kind: "out", text: "history prj_6jzec0m2nn rev 5 (showing 5 of 5, newest first)" },
			{ kind: "more", text: "r5 2026-11-20T21:57:39Z local:claude ops=5 caption.add,caption.add,caption.add,+2" },
			{ kind: "more", text: "r4 2026-11-20T21:57:30Z local:cli ops=9 track.add,track.add,track.add,+6" },
			{ kind: "tool", text: "Bash(stuarty export --project prj_6jzec0m2nn --wait)" },
			{ kind: "out", text: "job_zv5ar08jeb export succeeded 100% 1080x1920 22s/660f 21.9 MiB" },
		],
		copy: ["stuarty history --project prj_6jzec0m2nn", "stuarty export --project prj_6jzec0m2nn --wait"].join("\n"),
		doc: { label: "CLI reference", href: "/docs/reference/cli/" },
	},
	{
		id: "http",
		label: "HTTP API",
		title: "Claude Code calls the HTTP API",
		text: "The same operations as REST routes under /api/v1, with a bearer token. A batch can be checked with dry_run before it is applied.",
		lines: [
			{ kind: "cmd", text: "claude" },
			{ kind: "prompt", text: "With the HTTP API, try a caption at 14.5 s on Launch teaser without saving it." },
			{ kind: "tool", text: `Bash(${curlEdit})` },
			{ kind: "out", text: '{"project_id": "prj_6jzec0m2nn", "revision": 6, "base_revision": 5, "applied": false, "dry_run": true,' },
			{ kind: "more", text: '"actor": "local:api", "created": ["itm_b2jzp3byyr"], "effective": [...], "diff": [...]}' },
			{ kind: "tool", text: 'Bash(curl -s "http://127.0.0.1:7878/api/v1/projects" -H "Authorization: Bearer $STUARTY_KEY")' },
			{ kind: "out", text: '{"projects": [{"id": "prj_6jzec0m2nn", "name": "Launch teaser", "revision": 5, "width": 1080, "height": 1920, "fps": "30", ...}]}' },
		],
		copy: [curlEdit, 'curl -s "http://127.0.0.1:7878/api/v1/projects" -H "Authorization: Bearer $STUARTY_KEY"'].join("\n"),
		doc: { label: "API reference", href: "/docs/api/overview/" },
	},
	{
		id: "mcp",
		label: "MCP",
		title: "Twenty tools, grouped by intent",
		text: "stuarty mcp serves the tools on standard input and output. One edit call applies a whole batch as one undo step; history lists every step with who made it.",
		lines: [
			{ kind: "cmd", text: "claude mcp add stuarty -- stuarty mcp --actor claude" },
			{ kind: "note", text: "# 20 tools: project_list project_create project_get project_trash" },
			{ kind: "note", text: "#   edit command_help history media_import media_inspect view lint" },
			{ kind: "note", text: "#   job_start job_get job_cancel template font_import connection" },
			{ kind: "note", text: "#   image project_item library" },
			{ kind: "cmd", text: "claude" },
			{ kind: "prompt", text: "Who edited Launch teaser last?" },
			{ kind: "tool", text: 'stuarty - history (MCP)(project_id: "prj_6jzec0m2nn", action: "list", limit: 2)' },
			{ kind: "out", text: "history prj_6jzec0m2nn rev 5 (showing 2 of 5, newest first)" },
			{ kind: "more", text: "r5 2026-11-20T21:57:39Z local:claude ops=5 caption.add,caption.add,caption.add,+2" },
			{ kind: "more", text: "r4 2026-11-20T21:57:30Z local:cli ops=9 track.add,track.add,track.add,+6" },
		],
		copy: "claude mcp add stuarty -- stuarty mcp --actor claude",
		doc: { label: "MCP tools", href: "/docs/mcp/tools/" },
	},
];

export const trust = {
	eyebrow: "Yours",
	title: "On your machine. On your terms.",
	points: [
		{ title: "Open source", text: "MIT licensed. Read it, build it, change it." },
		{ title: "Self-hosted", text: "Web and desktop. Files, tokens and credentials stay with you." },
		{ title: "No telemetry", text: "With nothing connected, nothing leaves your machine." },
		{ title: "You Approve", text: "An agent can prepare a post. Only you can send it." },
		{ title: "A local vault for your keys and tokens", text: "Credentials live in the vault, and no tool ever returns one." },
		{ title: "Connect to External Services and APIs", text: "Your AI keys, Sign in with ChatGPT, OpenRouter or your local agent." },
	],
};

/** The closing take of the home, in the author's words, one line each. */
export const homeYours = {
	lines: ["On your machine. On your terms.", "No subscriptions. No pay once.", "Open Source"],
};

export const cta = {
	title: "Make the next one in one place.",
	lead: "Free and open source. Runs on Windows, macOS, Linux and your own server.",
	primary: { label: "Install Stuarty", href: "/docs/getting-started/quick-start/" },
	secondary: { label: "View on GitHub" },
};

export const credits = "Footage: Big Buck Bunny, (c) Blender Foundation, CC BY 3.0. Photo: CC0.";

/** The six takes of the viewport version (brainstorming section 5.1). */
export const takes = [
	{ id: "studio", name: "One place" },
	{ id: "timeline", name: "The timeline" },
	{ id: "agents", name: "Agents" },
	{ id: "yours", name: "Yours" },
] as const;

export type TimelineClip = {
	id: string;
	label: string;
	title: string;
	text: string;
	shot: Shot;
	/** How the picture comes in (MEDIA_FX in src/landing/timeline.ts); one per clip, none repeated. */
	fx: string;
	/** The main features of the screen, from the user guide and the interface (at least five). */
	features: string[];
};

/** The home timeline: the real flow of Stuarty, step by step, with every main screen of the dashboard; it ends on
 * publishing. */
export const homeTimeline: TimelineClip[] = [
	{
		id: "project",
		label: "new project",
		title: "Create a project.",
		text: "Name it and pick its size: vertical, square, landscape or portrait. A project holds everything of one production.",
		shot: shots.projects,
		fx: "fade",
		features: [
			"Four sizes: 9:16, 1:1, 16:9 and 4:5",
			"Each size names where it goes: Reels, Shorts, TikTok, YouTube, feeds",
			"Start from a template with slots",
			"Apply your brand kit colours and fonts to templates",
			"Rename, duplicate or move a project to the trash",
			"Restore from the trash, export or import a bundle",
		],
	},
	{
		id: "brief",
		label: "brief.md",
		title: "Answer the brief.",
		text: "Who it is for, what it should achieve, where it goes, how long, in which tone. The answers become the brief your agent reads.",
		shot: shots.projectBrief,
		fx: "slide-left",
		features: [
			"Guided questions become the brief of the production",
			"Audience, goal, platform, format, duration and tone",
			"Optional references, must-include, must-avoid and deadline",
			"YouTube Shorts, TikTok, Instagram Reels or Facebook Reels",
			"A duration in seconds, checked against a range",
			"Your agent reads the brief you saved",
		],
	},
	{
		id: "choose",
		label: "home",
		title: "Choose what to make.",
		text: "A video, an image or a text: every editor of the studio is one click away from the home.",
		shot: shots.dashboard,
		fx: "zoom-in",
		features: [
			"One home that links to every part of the work",
			"Each card says what its section holds",
			"The same pages in the browser and the desktop app",
			"A sidebar that folds to icons, a drawer on the phone",
			"Nothing goes online until you open Connections",
		],
	},
	{
		id: "write",
		label: "script.md",
		title: "Write the script.",
		text: "Scripts, copy and blog posts with revisions you can go back to, and how long the script takes read aloud.",
		shot: shots.writing,
		fx: "wipe-right",
		features: [
			"Scripts, copy, captions and blog posts per project",
			"A Markdown editor with a safe preview",
			"How long a script takes read aloud, at your pace",
			"Caption counters against the limit of each network",
			"Every save is a revision to compare or restore",
			"Draft with a model, after a cost estimate",
		],
	},
	{
		id: "media",
		label: "footage",
		title: "Bring in the footage.",
		text: "Video, music and voice-overs live in the workspace library, with where each came from, ready for any project.",
		shot: shots.videoAssets,
		fx: "slide-up",
		features: [
			"Video and audio with where each came from and where it is used",
			"Search by text, tag and origin",
			"Import MP4, MOV, MKV, WebM, MP3, WAV and more",
			"Generate a voice-over from a text",
			"Generate a video clip with a provider you connected",
			"Add a file to another project in one step",
		],
	},
	{
		id: "edit",
		label: "launch-teaser.mp4",
		title: "Cut it and add captions.",
		text: "Trim, split, transitions, colour and keyframes, with captions timed to every word.",
		shot: shots.videoEditor,
		fx: "swing",
		features: [
			"Split, trim the start and trim the end of a clip",
			"Fade, slide and crossfade transitions",
			"Captions timed to every word, generated from speech",
			"Music ducks under speech on its own",
			"Remove the silent stretches of a talking clip",
			"Export a verified MP4, GIF, WAV or AAC",
		],
	},
	{
		id: "thumbnail",
		label: "thumbnail.png",
		title: "Make the thumbnail.",
		text: "Layers, text, masks and templates in the image editor, with the same undo and history.",
		shot: shots.imageEditor,
		fx: "flip-y",
		features: [
			"Presets: 16:9 thumbnail, 1:1 post, 9:16 cover",
			"Fill, image, shape and text layers",
			"Adjustments and filters such as blur, glow and stroke",
			"Soft-edged masks, also from the shape of another layer",
			"Templates with slots you fill in",
			"Remove a background or generate an image, after your approval",
		],
	},
	{
		id: "images",
		label: "images",
		title: "Keep your images.",
		text: "Photos, logos and generated art stay in the library, tagged and reusable across projects.",
		shot: shots.visualAssets,
		fx: "circle",
		features: [
			"Images and colour tables with their origin and use",
			"Filter by origin: imported, generated, fetched by an agent",
			"Add and remove tags on any file",
			"Keep a rights note and mark a file reusable",
			"Generated images record the model and the prompt",
			"The same file imported twice is one entry",
		],
	},
	{
		id: "agent",
		label: "agent",
		title: "Bring your agent.",
		text: "Claude Code, Codex, OpenCode or Gemini CLI, with your own sign-in, working on the same project as you.",
		shot: shots.agentChat,
		fx: "blur",
		features: [
			"Hosts the ACP agent you already use and sign in to",
			"Profiles for Claude Code, Codex, OpenCode and Gemini CLI",
			"Replies, plans and tool calls stream in live",
			"Allow once or reject each tool request",
			"Its edits land in the history and can be undone",
			"Export the conversation as Markdown",
		],
	},
	{
		id: "skills",
		label: "skills",
		title: "Teach it your way.",
		text: "Skills are procedures your agent follows, reviewed by you before any agent may use them.",
		shot: shots.skills,
		fx: "flip-x",
		features: [
			"Procedures in the open Agent Skills format",
			"An imported skill waits until you mark it read",
			"Bundled scripts are stored, never run",
			"Lint a skill and read its findings",
			"Import from a folder or an archive, with no download",
			"Export reviewed skills for your own agent",
		],
	},
	{
		id: "prompts",
		label: "prompts",
		title: "Reuse your prompts.",
		text: "Texts with variables you fill in when you use them, for you and for your agents.",
		shot: shots.prompts,
		fx: "zoom-out",
		features: [
			"Reusable texts with {{variables}}",
			"Fill in the variables and render the text",
			"Rendering sends nothing anywhere",
			"A searchable list and a place to write new ones",
			"Edit and preview tabs, with tags",
			"Plain Markdown files in your workspace",
		],
	},
	{
		id: "memory",
		label: "memory",
		title: "Let it remember.",
		text: "One fact per note, read by your agents before they plan; a note an agent writes waits until you keep it.",
		shot: shots.memory,
		fx: "wipe-up",
		features: [
			"One fact per note, from past productions",
			"Agents read it before they plan the next one",
			"A note an agent writes waits until you keep it",
			"Search across memory, skills and prompts",
			"Plain files you can open, version and copy",
			"Rebuild the index from the files at any time",
		],
	},
	{
		id: "connect",
		label: "connections",
		title: "Connect your services.",
		text: "Your AI keys and your social accounts, through your own developer apps, stored in a local vault.",
		shot: shots.connections,
		fx: "diagonal",
		features: [
			"No telemetry; nothing leaves until you connect something",
			"Your own API key for each AI provider",
			"Sign in with ChatGPT or OpenRouter, with no key",
			"Credentials encrypted on this machine; no agent reads them",
			"YouTube, Instagram, Facebook, X and TikTok accounts",
			"Blog destinations: static site, WordPress, webhook or API",
		],
	},
	{
		id: "approve",
		label: "approvals",
		title: "Approve it.",
		text: "An agent can prepare a post or a paid generation. It waits here until you say yes.",
		shot: shots.approvals,
		fx: "skew",
		features: [
			"One queue for publications and costly generations",
			"The destination, the estimated cost and who asked",
			"Approve, or reject with a reason",
			"Rules that approve for you, within limits",
			"Cost ceilings per item and per day",
			"An agent can never approve or raise a ceiling",
		],
	},
	{
		id: "publish",
		label: "publish",
		title: "Publish on the calendar.",
		text: "Plan each post for your own accounts and pick its time. Nothing goes out before your approval.",
		shot: shots.calendar,
		fx: "iris",
		features: [
			"Month, week and list views of what is planned",
			"Drag a post, or move it with the keyboard",
			"One publication, one target per account with its own time",
			"Every post waits for your approval first",
			"Time zones handled; a missed post is never sent on its own",
			"Publish videos or blog posts, and retry a failure",
		],
	},
];
