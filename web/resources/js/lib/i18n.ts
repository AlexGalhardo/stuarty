import {
	createContext,
	createElement,
	type ReactNode,
	useContext,
} from "react";

// The catalog the server sends: inertia.I18n("ui.") for Inertia pages, the
// bootstrap object for the editor (SDD-012 section 16.3).
export type Message = string | Record<string, string>;

export interface I18nProps {
	locale: string;
	locales: string[];
	messages: Record<string, Message>;
	[key: string]: unknown;
}

export type Translate = (
	key: string,
	params?: Record<string, string | number>,
) => string;

// translate fills {name} placeholders and picks plural forms; a missing key
// returns the key itself, as the server does, so a typo shows on screen.
export function translate(
	messages: Record<string, Message>,
	locale: string,
	key: string,
	params: Record<string, string | number> = {},
): string {
	let msg = messages[key];
	if (msg === undefined) return key;
	if (typeof msg !== "string") {
		const n = Number(params.count);
		const category = new Intl.PluralRules(locale).select(n);
		msg = (n === 0 && msg.zero) || msg[category] || msg.other || key;
	}
	return msg.replace(/\{(\w+)\}/g, (match, name: string) =>
		name in params ? String(params[name]) : match,
	);
}

export interface I18nContextValue {
	messages: Record<string, Message>;
	locale: string;
	// The languages the server offers, for the language select.
	locales?: readonly string[];
}

const I18nContext = createContext<I18nContextValue>({
	messages: {},
	locale: "en",
});

// I18nProvider feeds useT. Inertia pages get it from AppLayout (props of the
// page); the editor gets it from its bootstrap, so shared components such as
// ThemeToggle serve both without importing Inertia.
export function I18nProvider({
	messages,
	locale,
	locales,
	children,
}: I18nContextValue & { children?: ReactNode }): ReactNode {
	return createElement(
		I18nContext.Provider,
		{ value: { messages, locale, locales } },
		children,
	);
}

export function useT(): Translate {
	const { messages, locale } = useContext(I18nContext);
	return (key, params = {}) => translate(messages, locale, key, params);
}

export function useLocale(): string {
	return useContext(I18nContext).locale;
}

export function useLocales(): readonly string[] {
	const { locale, locales } = useContext(I18nContext);
	return locales ?? [locale];
}
