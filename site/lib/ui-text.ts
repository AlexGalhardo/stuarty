import type { Locale } from "@/lib/locale";

/** Interface strings of the site chrome per locale (SDD-010 section 4.4). A key missing in one locale is a
 * type error. Client-safe: no file system access here. */
export type UiText = {
	skipToContent: string;
	logoLabel: string;
	home: string;
	docs: string;
	githubTooltip: string;
	githubLabel: string;
	agents: string;
	changelog: string;
	changelogTitle: string;
	changelogDescription: string;
	openNavigation: string;
	closeNavigation: string;
	navigation: string;
	documentation: string;
	onThisPage: string;
	releases: string;
	colorTheme: string;
	lightTheme: string;
	darkTheme: string;
	systemTheme: string;
	lastUpdated: string;
	editOnGitHub: string;
	pagination: string;
	previous: string;
	next: string;
	language: string;
	fallbackNotice: string;
	generatedNote: string;
	changelogNote: string;
	notFoundTitle: string;
	notFoundBody: string;
	goToIntroduction: string;
	searchDocs: string;
	searchPlaceholder: string;
	searchButton: string;
	noResults: string;
	searchFailed: string;
	copied: string;
};

export const ui: Record<Locale, UiText> = {
	en: {
		skipToContent: "Skip to content",
		logoLabel: "Vukem Video home",
		home: "Home",
		docs: "Docs",
		githubTooltip: "GitHub repository",
		githubLabel: "Vukem Video on GitHub",
		agents: "Agents",
		changelog: "Changelog",
		changelogTitle: "Changelog",
		changelogDescription: "Every release of Vukem Video, with a link to the commit behind each change.",
		openNavigation: "Open navigation",
		closeNavigation: "Close navigation",
		navigation: "Navigation",
		documentation: "Documentation",
		onThisPage: "On this page",
		releases: "Releases",
		colorTheme: "Color theme",
		lightTheme: "Light theme",
		darkTheme: "Dark theme",
		systemTheme: "System theme",
		lastUpdated: "Last updated",
		editOnGitHub: "Edit this page on GitHub",
		pagination: "Pagination",
		previous: "Previous",
		next: "Next",
		language: "Language",
		fallbackNotice: "This page has not been translated yet. You are reading the English version.",
		generatedNote: "This reference is generated from the program and is in English.",
		changelogNote: "Release notes are written in English.",
		notFoundTitle: "Page not found",
		notFoundBody: "The page you are looking for does not exist or has moved.",
		goToIntroduction: "Go to the introduction",
		searchDocs: "Search documentation",
		searchPlaceholder: "Search the docs…",
		searchButton: "Search…",
		noResults: "No results for",
		searchFailed: "Search index could not be loaded.",
		copied: "Copied to clipboard",
	},
	"pt-br": {
		skipToContent: "Ir para o conteúdo",
		logoLabel: "Página inicial do Vukem Video",
		home: "Início",
		docs: "Documentação",
		githubTooltip: "Repositório no GitHub",
		githubLabel: "Vukem Video no GitHub",
		agents: "Agentes",
		changelog: "Changelog",
		changelogTitle: "Changelog",
		changelogDescription: "Todas as versões do Vukem Video, com um link para o commit de cada mudança.",
		openNavigation: "Abrir a navegação",
		closeNavigation: "Fechar a navegação",
		navigation: "Navegação",
		documentation: "Documentação",
		onThisPage: "Nesta página",
		releases: "Versões",
		colorTheme: "Tema de cores",
		lightTheme: "Tema claro",
		darkTheme: "Tema escuro",
		systemTheme: "Tema do sistema",
		lastUpdated: "Última atualização",
		editOnGitHub: "Editar esta página no GitHub",
		pagination: "Paginação",
		previous: "Anterior",
		next: "Próxima",
		language: "Idioma",
		fallbackNotice: "Esta página ainda não foi traduzida. Você está lendo a versão em inglês.",
		generatedNote: "Esta referência é gerada a partir do programa e está em inglês.",
		changelogNote: "As notas de versão são escritas em inglês.",
		notFoundTitle: "Página não encontrada",
		notFoundBody: "A página que você procura não existe ou mudou de endereço.",
		goToIntroduction: "Ir para a introdução",
		searchDocs: "Pesquisar na documentação",
		searchPlaceholder: "Pesquisar na documentação…",
		searchButton: "Pesquisar…",
		noResults: "Nenhum resultado para",
		searchFailed: "Não foi possível carregar o índice de busca.",
		copied: "Copiado para a área de transferência",
	},
};

/** "Last updated" timestamp, printed in UTC. */
export function formatUpdated(iso: string, locale: Locale = "en"): string {
	return new Intl.DateTimeFormat(locale === "en" ? "en" : "pt-BR", {
		year: "numeric",
		month: "long",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
		timeZone: "UTC",
		timeZoneName: "short",
	}).format(new Date(iso));
}

/** The title and description of the changelog page in a locale. */
export function changelogMetadataOf(locale: Locale): { title: string; description: string } {
	return { title: ui[locale].changelogTitle, description: ui[locale].changelogDescription };
}
