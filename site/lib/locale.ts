/** The locales of the site (SDD-010 section 2.1). Client-safe: no file system access here. */
export const locales = ["en", "pt-br"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "en";

/** A string in every locale. */
export type LocalizedText = Record<Locale, string>;

/** The `lang` attribute: BCP 47 casing, while paths and files use the lower-case code. */
export const htmlLang: Record<Locale, string> = { en: "en", "pt-br": "pt-BR" };

/** The URL prefix of a locale: none for English, so existing addresses keep working. */
export function localePrefix(locale: Locale): string {
	return locale === "en" ? "" : `/${locale}`;
}

const LOCALIZED = /^\/(?:docs|changelog|llms|search-index)(?:[/.#?]|$)|^\/$/;

/** An internal path of the English tree (`/docs/x`) as it is served in a locale; other paths are unchanged. */
export function localizePath(locale: Locale, path: string): string {
	if (locale === "en" || !LOCALIZED.test(path)) return path;
	return path === "/" ? `${localePrefix(locale)}/` : `${localePrefix(locale)}${path}`;
}

/** The locale a served path belongs to. */
export function localeOfPath(path: string): Locale {
	return /^\/pt-br(?:\/|$)/.test(path) ? "pt-br" : "en";
}

/** A served path without its locale prefix. */
export function stripLocale(path: string): string {
	return path.replace(/^\/pt-br(?=\/|$)/, "") || "/";
}

export function otherLocale(locale: Locale): Locale {
	return locale === "en" ? "pt-br" : "en";
}
