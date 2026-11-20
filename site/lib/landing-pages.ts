/**
 * Copy of the three candidate landing pages (/landing/viewport, /landing/classic, /landing/creative), English only.
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
	/** Lines of the sample; `cmd` lines are typed, `out` lines are the answer, `note` lines are comments. */
	lines: { kind: "cmd" | "out" | "note"; text: string }[];
	/** What the copy button copies (the commands only). */
	copy: string;
	doc: { label: string; href: string };
};

const projectId = "prj_6jzec0m2nn";

export const drive: DriveTab[] = [
	{
		id: "agent",
		label: "Agent",
		title: "Your agent edits beside you",
		text: "Connect the agent you already use. It reads the project, plans a batch, lints and exports, and every change appears in the timeline signed with its name.",
		lines: [
			{ kind: "cmd", text: "claude mcp add stuarty -- stuarty mcp" },
			{ kind: "note", text: "> Add the four captions of the script to Launch teaser, then export it." },
			{ kind: "out", text: `project_get   ${projectId}  rev 4, 4 tracks, 22 s` },
			{ kind: "out", text: "edit          caption.add x4, item.set_transition  rev 5, actor local:claude" },
			{ kind: "out", text: "lint          0 errors" },
			{ kind: "out", text: "job_start     kind=export  job_zv5ar08jeb" },
			{ kind: "out", text: "job_get       succeeded  1080x1920  22s/660f  21.9 MiB" },
		],
		copy: "claude mcp add stuarty -- stuarty mcp",
		doc: { label: "Agents guide", href: "/docs/mcp/overview/" },
	},
	{
		id: "cli",
		label: "CLI",
		title: "Every action is a command",
		text: "The same operations as the editor, from a terminal or a script. Add --json to any command for one JSON document.",
		lines: [
			{ kind: "cmd", text: 'stuarty project:create --name "Launch teaser" --preset 9:16' },
			{ kind: "out", text: `created ${projectId} "Launch teaser" r0 1080x1920 30fps; add tracks with edit op track.add` },
			{ kind: "cmd", text: `stuarty media:import talk.mp4 --project ${projectId} --wait` },
			{ kind: "cmd", text: `stuarty edit --project ${projectId} caption.add start=0.3s text="One studio for everything you make" duration=3s` },
			{ kind: "cmd", text: `stuarty lint --project ${projectId}` },
			{ kind: "cmd", text: `stuarty export --project ${projectId} --wait` },
			{ kind: "out", text: "job_zv5ar08jeb export succeeded 100% 1080x1920 22s/660f 21.9 MiB" },
		],
		copy: [
			'stuarty project:create --name "Launch teaser" --preset 9:16',
			`stuarty media:import talk.mp4 --project ${projectId} --wait`,
			`stuarty edit --project ${projectId} caption.add start=0.3s text="One studio for everything you make" duration=3s`,
			`stuarty lint --project ${projectId}`,
			`stuarty export --project ${projectId} --wait`,
		].join("\n"),
		doc: { label: "CLI reference", href: "/docs/reference/cli/" },
	},
	{
		id: "http",
		label: "HTTP API",
		title: "REST routes with an event stream",
		text: "The API lives under /api/v1 with a bearer token. Edits are batches against a revision, and the events route streams every change as it lands.",
		lines: [
			{ kind: "cmd", text: 'curl -X POST "http://127.0.0.1:7878/api/v1/projects" -H "Authorization: Bearer $STUARTY_KEY" -H "Content-Type: application/json" -d \'{"name": "Launch teaser", "preset": "9:16"}\'' },
			{ kind: "out", text: "201 Created" },
			{ kind: "cmd", text: `curl -X POST "http://127.0.0.1:7878/api/v1/projects/${projectId}/edit" -H "Authorization: Bearer $STUARTY_KEY" -H "Content-Type: application/json" -d '{"base_revision": 4, "operations": [{"op": "caption.add", "args": {"start": "0.3s", "text": "One studio", "duration": "3s"}}]}'` },
			{ kind: "cmd", text: `curl -N "http://127.0.0.1:7878/api/v1/projects/${projectId}/events" -H "Authorization: Bearer $STUARTY_KEY"` },
		],
		copy: [
			'curl -X POST "http://127.0.0.1:7878/api/v1/projects" -H "Authorization: Bearer $STUARTY_KEY" -H "Content-Type: application/json" -d \'{"name": "Launch teaser", "preset": "9:16"}\'',
			`curl -N "http://127.0.0.1:7878/api/v1/projects/${projectId}/events" -H "Authorization: Bearer $STUARTY_KEY"`,
		].join("\n"),
		doc: { label: "API reference", href: "/docs/api/overview/" },
	},
	{
		id: "mcp",
		label: "MCP",
		title: "Tools grouped by intent",
		text: "tools/list returns 20 tools: read, edit, view, lint and run jobs. One edit call applies a whole batch of operations as one undo step.",
		lines: [
			{ kind: "note", text: "tools/call" },
			{ kind: "out", text: '{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {' },
			{ kind: "out", text: '  "name": "edit", "arguments": {' },
			{ kind: "out", text: `    "project_id": "${projectId}", "base_revision": 4,` },
			{ kind: "out", text: '    "operations": [{"op": "caption.add", "args": {"start": "0.3s",' },
			{ kind: "out", text: '      "text": "One studio for everything you make", "duration": "3s"}}]}}}' },
		],
		copy: `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "edit", "arguments": {"project_id": "${projectId}", "base_revision": 4, "operations": [{"op": "caption.add", "args": {"start": "0.3s", "text": "One studio for everything you make", "duration": "3s"}}]}}}`,
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
		{ title: "A person approves", text: "An agent can prepare a post. Only you can send it." },
		{ title: "A local vault", text: "Credentials live in the vault, and no tool ever returns one." },
		{ title: "Bring your own access", text: "Your AI keys, Sign in with ChatGPT, OpenRouter or your local agent." },
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
	{ id: "video", name: "Edit video" },
	{ id: "images", name: "Images and writing" },
	{ id: "publish", name: "Publish" },
	{ id: "agents", name: "Agents" },
	{ id: "yours", name: "Yours" },
] as const;
