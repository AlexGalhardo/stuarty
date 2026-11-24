import type { ReactNode } from "react";
import { renderToString } from "react-dom/server";

import { I18nProvider, type Message } from "@/lib/i18n";
import en from "../../lang/en.json";
import pt from "../../lang/pt-BR.json";

// The catalogue the server sends is the flattened "ui." tree, with a plural kept as one object (lib/i18n.ts).
function flat(
	tree: Record<string, unknown>,
	prefix: string,
	out: Record<string, Message> = {},
): Record<string, Message> {
	for (const [key, value] of Object.entries(tree)) {
		if (typeof value === "string") {
			out[`${prefix}${key}`] = value;
			continue;
		}
		const object = value as Record<string, unknown>;
		if (typeof object.other === "string") {
			out[`${prefix}${key}`] = object as Record<string, string>;
		} else flat(object, `${prefix}${key}.`, out);
	}
	return out;
}

const CATALOGUES = { en: flat(en.ui, "ui."), "pt-BR": flat(pt.ui, "ui.") };

// Renders a node the way the page does, with the real catalogue of a language, so a test reads the words a person
// reads and a key that is missing shows as the key.
export function drawForTest(
	node: ReactNode,
	locale: keyof typeof CATALOGUES = "en",
): string {
	return renderToString(
		<I18nProvider messages={CATALOGUES[locale]} locale={locale}>
			{node}
		</I18nProvider>,
	);
}
