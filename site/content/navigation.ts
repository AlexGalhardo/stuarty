/**
 * Sidebar navigation. Each category maps to a folder in content/docs and
 * lists its pages in display order. A page "<dir>/<slug>.mdx" is served at
 * /docs/<prefix>/<slug> (prefix defaults to the folder name; the
 * "Getting started" pages live at the root of /docs).
 */
export type NavCategory = {
	title: string;
	dir: string;
	prefix?: string;
	pages: string[];
};

export const navigation: NavCategory[] = [
	{
		title: "Getting started",
		dir: "getting-started",
		prefix: "",
		pages: ["introduction", "quick-start"],
	},
];
