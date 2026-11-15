/** Landing page copy, English first. Both locales have the same shape; the page is plain static HTML. */
export type Locale = "en" | "pt-br";

export type LandingCopy = {
	lang: "en" | "pt-BR";
	title: string;
	description: string;
	nav: { docs: string; github: string; releases: string; other: { label: string; href: string } };
	status: string;
	hero: { title: string; lead: string; primary: string; secondary: string; dockerLabel: string; dockerNote: string };
	tour: { title: string; lead: string; items: { title: string; text: string; phase: string }[] };
	agents: {
		title: string;
		lead: string;
		routes: { name: string; command: string; text: string }[];
		note: string;
	};
	open: { title: string; points: { title: string; text: string }[] };
	footer: { docs: string; repo: string; changelog: string; license: string; docsNote?: string };
};

export const dockerCommand = "docker run -p 127.0.0.1:7878:7878 ghcr.io/alexgalhardo/vukem-video";

export const landing: Record<Locale, LandingCopy> = {
	en: {
		lang: "en",
		title: "Vukem Video: an open source video editor that agents and people share",
		description:
			"A minimalist, local-first, open source (MIT) video editor. An AI agent edits through MCP, a CLI or an HTTP API; you edit the same project in the browser. In the BETA phase.",
		nav: {
			docs: "Docs",
			github: "GitHub",
			releases: "Releases",
			other: { label: "Português", href: "/pt-br/" },
		},
		status: "BETA phase: released and still before 1.0. Each release is on the releases page.",
		hero: {
			title: "Edit video with an agent, or by hand. Same project, same result.",
			lead: "Vukem Video is a minimalist, open source video editor for short vertical videos with captions and for product commercials. An AI agent drives it through MCP, a CLI or an HTTP API, and you open the same project in your browser.",
			primary: "Run it",
			secondary: "Read the docs",
			dockerLabel: "Run it on your machine with one command:",
			dockerNote: "The server listens on localhost only.",
		},
		tour: {
			title: "What the editor does",
			lead: "A small set of operations, done well, that an agent and a person can both use. Each release adds more; the changelog lists what.",
			items: [
				{
					title: "Multi-track timeline",
					text: "Tracks, clips and text. Trim, split, move and delete with ripple. Undo and redo for each actor, as many steps as you need.",
					phase: "Released",
				},
				{
					title: "Captions burned in",
					text: "Timed caption items with one style per track, for the vertical videos social networks expect. 9:16 is the default frame.",
					phase: "Released",
				},
				{
					title: "Exact preview, verified export",
					text: "The paused preview is the frame the export produces. An MP4 export reports success only after both streams and the duration are checked.",
					phase: "Released",
				},
				{
					title: "Automatic transcription",
					text: "Turn the speech of a clip into caption items, with per-word timing and a highlight for the word being spoken.",
					phase: "Released",
				},
				{
					title: "Templates for commercials",
					text: "Save a project as a template with named slots for media, text and colours, then fill the slots for the next product.",
					phase: "Released",
				},
				{
					title: "Transitions and keyframes",
					text: "Fades, slides, crossfades and keyframes over position, scale, rotation and opacity, with easing.",
					phase: "Released",
				},
			],
		},
		agents: {
			title: "Built for agents as first-class users",
			lead: "Every operation is one command in one core written in Go. The agent interfaces and the browser UI are thin adapters over it, so a human and an agent see the same state and the same history.",
			routes: [
				{
					name: "MCP",
					command: "vukem-video mcp",
					text: "A stdio MCP server for Claude Code, Codex and OpenCode, or Streamable HTTP at POST /mcp.",
				},
				{
					name: "CLI",
					command: 'vukem-video project:create --name "Launch teaser" --json',
					text: "Machine-readable output on every command.",
				},
				{
					name: "HTTP API",
					command: "vukem-video serve",
					text: "The same commands over HTTP on 127.0.0.1:7878.",
				},
			],
			note: "The same commands run through all three.",
		},
		open: {
			title: "Open source and local-first",
			points: [
				{
					title: "MIT licensed",
					text: "Read it, run it, change it. The code and the specifications are public.",
				},
				{
					title: "Runs on your machine",
					text: "No account and no cloud service. The container or installer starts a server that listens on localhost.",
				},
				{
					title: "Nothing leaves it",
					text: "No outbound network connection by default and no telemetry. Agents cannot make the product send your media anywhere.",
				},
				{
					title: "FFmpeg kept apart",
					text: "FFmpeg runs only as a separate LGPL process. The pixels are composed in Go.",
				},
			],
		},
		footer: {
			docs: "Documentation",
			repo: "Repository",
			changelog: "Changelog",
			license: "MIT license. Copyright 2026 Alex Galhardo.",
		},
	},
	"pt-br": {
		lang: "pt-BR",
		title: "Vukem Video: um editor de vídeo de código aberto para agentes e pessoas",
		description:
			"Um editor de vídeo minimalista, local e de código aberto (MIT). Um agente de IA edita por MCP, CLI ou API HTTP; você edita o mesmo projeto no navegador. Em fase BETA.",
		nav: {
			docs: "Documentação",
			github: "GitHub",
			releases: "Versões",
			other: { label: "English", href: "/" },
		},
		status: "Fase BETA: já publicado e ainda antes da 1.0. Cada versão está na página de versões.",
		hero: {
			title: "Edite vídeo com um agente ou à mão. O mesmo projeto, o mesmo resultado.",
			lead: "O Vukem Video é um editor de vídeo minimalista e de código aberto para vídeos verticais curtos com legendas e para vídeos de produto. Um agente de IA o conduz por MCP, CLI ou API HTTP, e você abre o mesmo projeto no navegador.",
			primary: "Executar",
			secondary: "Ler a documentação",
			dockerLabel: "Rode na sua máquina com um comando:",
			dockerNote: "O servidor escuta apenas em localhost.",
		},
		tour: {
			title: "O que o editor faz",
			lead: "Um conjunto pequeno de operações, bem feitas, que um agente e uma pessoa podem usar. Cada versão traz mais; o changelog lista o quê.",
			items: [
				{
					title: "Linha do tempo com várias trilhas",
					text: "Trilhas, clipes e texto. Corte, divida, mova e apague com ripple. Desfazer e refazer por autor, quantos passos precisar.",
					phase: "Publicado",
				},
				{
					title: "Legendas gravadas na imagem",
					text: "Itens de legenda com tempo e um estilo por trilha, para os vídeos verticais que as redes sociais pedem. 9:16 é o quadro padrão.",
					phase: "Publicado",
				},
				{
					title: "Prévia exata, exportação verificada",
					text: "A prévia pausada é o quadro que a exportação produz. A exportação em MP4 só é dada como concluída depois de checar os dois fluxos e a duração.",
					phase: "Publicado",
				},
				{
					title: "Transcrição automática",
					text: "Transforme a fala de um clipe em legendas, com tempo por palavra e destaque da palavra falada.",
					phase: "Publicado",
				},
				{
					title: "Modelos para vídeos de produto",
					text: "Salve um projeto como modelo com campos nomeados para mídia, texto e cores, e preencha os campos para o próximo produto.",
					phase: "Publicado",
				},
				{
					title: "Transições e keyframes",
					text: "Fades, deslizes, fusões e keyframes de posição, escala, rotação e opacidade, com suavização.",
					phase: "Publicado",
				},
			],
		},
		agents: {
			title: "Feito para agentes como usuários de primeira classe",
			lead: "Cada operação é um comando em um único núcleo escrito em Go. As interfaces de agente e a interface web são adaptadores finos sobre ele, então uma pessoa e um agente veem o mesmo estado e o mesmo histórico.",
			routes: [
				{
					name: "MCP",
					command: "vukem-video mcp",
					text: "Um servidor MCP stdio para Claude Code, Codex e OpenCode, ou HTTP em streaming em POST /mcp.",
				},
				{
					name: "CLI",
					command: 'vukem-video project:create --name "Launch teaser" --json',
					text: "Saída legível por máquina em todo comando.",
				},
				{
					name: "API HTTP",
					command: "vukem-video serve",
					text: "Os mesmos comandos por HTTP em 127.0.0.1:7878.",
				},
			],
			note: "Os mesmos comandos funcionam pelas três.",
		},
		open: {
			title: "Código aberto e local",
			points: [
				{ title: "Licença MIT", text: "Leia, rode, altere. O código e as especificações são públicos." },
				{
					title: "Roda na sua máquina",
					text: "Sem conta e sem serviço em nuvem. O contêiner ou o instalador inicia um servidor que escuta em localhost.",
				},
				{
					title: "Nada sai dela",
					text: "Nenhuma conexão de saída por padrão e nenhuma telemetria. Agentes não conseguem fazer o produto enviar sua mídia a lugar nenhum.",
				},
				{
					title: "FFmpeg à parte",
					text: "O FFmpeg roda apenas como um processo LGPL separado. Os pixels são compostos em Go.",
				},
			],
		},
		footer: {
			docs: "Documentação",
			repo: "Repositório",
			changelog: "Changelog",
			license: "Licença MIT. Copyright 2026 Alex Galhardo.",
			docsNote: "A documentação está apenas em inglês por enquanto.",
		},
	},
};
