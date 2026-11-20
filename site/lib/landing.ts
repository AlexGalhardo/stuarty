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
	title: "Stuarty: create, edit, manage and publish your content in one place",
	description:
		"An open source (MIT), self-hosted studio for creators: video, images and writing, with AI agents as first-class users. Your files and credentials stay on your machine, and nothing is published until you approve it.",
	hero: {
		title: "Create, edit and publish in one place.",
		lead: "A studio for independent creators: edit a short vertical video with captions, make its thumbnail, write the post and publish it. An agent can do everything you can, through MCP, a CLI or an HTTP API, and you work on the same project in your browser.",
		runLabel: "Try it with Docker. One command starts the latest release:",
		runNote: "Needs Docker and a bash or zsh shell. The port is published on this computer only. Then open http://127.0.0.1:7878 and sign in with that email and password. With nothing connected, nothing leaves your machine; a post goes out only after you approve it.",
		points: [
			"Files, tokens and credentials stay on your machine.",
			"Nothing leaves until you connect a destination.",
			"Publishing waits for your approval.",
			"No telemetry. Free and open source (MIT).",
		],
		quickStart: "Make your first short video",
		releases: "Releases",
	},
	gallery: {
		label: "Stuarty in action",
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
			id: "image",
			tab: "Thumbnails",
			title: "Thumbnails from templates",
			text: "Make a cover in the image editor: layers, text, adjustments, filters and masks, saved as a template with slots. An agent works on the same image, with the same undo.",
			command: "image:create --from-template",
			image: {
				src: "/placeholders/compositing.svg",
				alt: "Placeholder for a screenshot of the image editor: a vertical cover with a title layer and the layers panel.",
			},
		},
		{
			id: "agents",
			tab: "Agents",
			title: "Agents at the keyboard",
			text: "An agent works in the same project through MCP, the CLI or the HTTP API: video, images, writing and publishing. The editor shows what changed and who changed it, and every edit can be undone.",
			command: "stuarty mcp",
			image: {
				src: "/placeholders/agents.svg",
				alt: "Placeholder for a screenshot: a terminal where an agent runs editing commands while the editor updates.",
			},
		},
		{
			id: "export",
			tab: "Publish",
			title: "Publish what you approved",
			text: "Export with a preset for the network, check the post against its limits, and schedule it. It goes out only after you approve it, through your own developer app. Nothing is sent before.",
			command: "stuarty publication:schedule",
			image: {
				src: "/placeholders/export.svg",
				alt: "Placeholder for a screenshot of the Approvals page: a post waiting for approval with its estimated cost.",
			},
		},
	] satisfies Feature[],
};
