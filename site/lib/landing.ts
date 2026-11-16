/** Copy of the landing page (English only). The Docker command runs the image tagged latest. */

export type Feature = {
	id: string;
	/** Short label of the tab. */
	tab: string;
	title: string;
	text: string;
	/** A command or operation the feature is driven by, shown under the text. */
	command: string;
	image: { src: string; alt: string };
};

export const landing = {
	title: "Vukem Video: an open source video editor that agents and people share",
	description:
		"A minimalist, local-first, open source (MIT) video editor. An AI agent edits through MCP, a CLI or an HTTP API; you edit the same project in the browser.",
	hero: {
		title: "Edit video with an agent, or by hand.",
		lead: "A minimalist, open source video editor for short vertical videos with captions and for product commercials. An agent drives it through MCP, a CLI or an HTTP API; you edit the same project in your browser.",
		runLabel: "Try it with Docker. One command starts the latest release:",
		runNote: "Needs Docker and a bash or zsh shell. The port is published on this computer only. Then open http://127.0.0.1:7878 and sign in with that email and password.",
		points: [
			"Agents and people share one project.",
			"One command core behind every interface.",
			"Local first, no telemetry.",
			"Free and open source (MIT).",
		],
		quickStart: "Read the quick start",
		releases: "Releases",
	},
	gallery: {
		label: "The editor in action",
		placeholderNote: "Placeholder images: real screenshots replace them in a later release.",
	},
	features: [
		{
			id: "captions",
			tab: "Captions",
			title: "Captions that follow the voice",
			text: "Turn the speech of a clip into caption items with per-word timing, then style them once for the whole track: font, colors and a highlight on the word being spoken.",
			command: "caption.from_transcript",
			image: {
				src: "/placeholders/captions.svg",
				alt: "Placeholder for a screenshot of the editor: a vertical video with word-timed captions and the caption style in the inspector.",
			},
		},
		{
			id: "cuts",
			tab: "Cuts",
			title: "Cut, trim and move",
			text: "Split a clip, trim its ends, move it, close the gap, slip its content, change its speed or remove the silences. Every actor, human or agent, has its own undo and redo.",
			command: "item.split, item.trim, clip.remove_silences",
			image: {
				src: "/placeholders/cuts.svg",
				alt: "Placeholder for a screenshot of the editor: the multi-track timeline while a clip is split and trimmed.",
			},
		},
		{
			id: "compositing",
			tab: "Chroma key and masks",
			title: "Chroma key, blend modes and masks",
			text: "Key out a green screen, blend a layer with the ones below it and shape it with a mask, with keyframes over position, scale, rotation and opacity.",
			command: "clip.set_chroma_key, clip.set_blend, clip.set_mask",
			image: {
				src: "/placeholders/compositing.svg",
				alt: "Placeholder for a screenshot of the editor: a clip over a green background with chroma key, blend mode and mask settings.",
			},
		},
		{
			id: "agents",
			tab: "Agents",
			title: "Agents at the keyboard",
			text: "An agent edits the same project through MCP, the CLI or the HTTP API. The editor shows what changed and who changed it, and every edit can be undone.",
			command: "vukem-video mcp",
			image: {
				src: "/placeholders/agents.svg",
				alt: "Placeholder for a screenshot: a terminal where an agent runs editing commands while the editor updates.",
			},
		},
		{
			id: "export",
			tab: "Export",
			title: "Export what you previewed",
			text: "Export MP4, GIF, WAV, AAC or a single PNG frame. An MP4 export reports success only after the file is checked: both streams and the duration.",
			command: "vukem-video export --format mp4",
			image: {
				src: "/placeholders/export.svg",
				alt: "Placeholder for a screenshot of the editor: the export dialog with a progress bar.",
			},
		},
	] satisfies Feature[],
};
