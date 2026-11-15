import { createContext, useContext } from "react";
import { defaultLocale, type Locale } from "@/lib/locale";
import { ui, type UiText } from "@/lib/ui-text";

const LocaleContext = createContext<Locale>(defaultLocale);

/** Gives the components below it the locale of the page: links, strings and dates follow it. */
export function LocaleProvider({ locale, children }: { locale: Locale; children: React.ReactNode }) {
	return <LocaleContext.Provider value={locale}>{children}</LocaleContext.Provider>;
}

export function useLocale(): Locale {
	return useContext(LocaleContext);
}

/** The interface strings of the current locale. */
export function useT(): UiText {
	return ui[useContext(LocaleContext)];
}
