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
		height: 820,
	},
	calendar: {
		src: "/landing/publish-calendar",
		alt: "The Publish section: the month view of the publishing calendar, with a note that every post waits for your approval before it is sent.",
		width: 1600,
		height: 780,
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
		height: 780,
	},
	dashboard: {
		src: "/landing/dashboard",
		alt: "The Stuarty home: projects, the video and image editors, writing, visual and video assets, publishing, skills, prompts, memory and connections.",
		width: 1600,
		height: 780,
	},
} satisfies Record<string, Shot>;

export const hero = {
	/** The title of the home page (the author's wording); the backup versions keep `title`. */
	homeTitle: "Create, edit, manage and publish your audiovisual and text content in one place",
	eyebrow: "Open source studio for creators",
	title: "Create, edit, manage and publish in one place.",
	lead: "Script, cut, caption, thumbnail and post from one studio on your own machine. Your AI agent works beside you, through the same commands you use.",
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

export const cta = {
	title: "Make the next one in one place.",
	lead: "Free and open source. Runs on Windows, macOS, Linux and your own server.",
	primary: { label: "Install Stuarty", href: "/docs/getting-started/quick-start/" },
	secondary: { label: "View on GitHub" },
};

export const credits = "Footage: Big Buck Bunny, (c) Blender Foundation, CC BY 3.0. Photo: CC0. Clips are placeholders until the real recordings arrive.";

/** The six takes of the viewport version (brainstorming section 5.1). */
export const takes = [
	{ id: "studio", name: "One place" },
	{ id: "timeline", name: "The timeline" },
	{ id: "agents", name: "Agents" },
	{ id: "yours", name: "Yours" },
] as const;
