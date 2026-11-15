import type { Translate } from "@/lib/i18n";

// The localized headline of an error code (SDD-012 section 4.4); a code with
// no catalogue key falls back to the generic headline.
export function errorHeadline(t: Translate, code: string): string {
	const key = `ui.editor.errors.${code}`;
	const text = t(key);
	return text === key ? t("ui.editor.errors.unknown") : text;
}
