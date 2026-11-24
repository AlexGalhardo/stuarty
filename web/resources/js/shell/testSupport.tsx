import type { ReactNode } from "react";

import { I18nProvider } from "@/lib/i18n";
import en from "../../lang/en.json";
import { ShellProvider } from "./ShellContext";

// The catalogue the server sends is the flattened "ui." tree; the tests of the shell read the English one.
function flat(
	tree: Record<string, unknown>,
	prefix: string,
	out: Record<string, string> = {},
): Record<string, string> {
	for (const [key, value] of Object.entries(tree)) {
		if (typeof value === "string") out[`${prefix}${key}`] = value;
		else flat(value as Record<string, unknown>, `${prefix}${key}.`, out);
	}
	return out;
}

export const messages = flat(en.ui, "ui.");

export function Frame({ children }: { children: ReactNode }): ReactNode {
	return (
		<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
			<ShellProvider>{children}</ShellProvider>
		</I18nProvider>
	);
}
