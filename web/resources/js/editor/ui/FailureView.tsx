import { CircleAlert } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useLocale, useT } from "@/lib/i18n";
import { localizeError } from "@/lib/localizeError";

// What the editor shows for an error: the code, the message and the hint of the
// core when it is a core error (SDD-012 sections 4.4 and 5.6), never a blank page.
export interface Failure {
	code?: string;
	message: string;
	hint?: string;
	details?: { readonly [key: string]: unknown };
}

export function FailureView({ failure }: { failure: Failure }): ReactNode {
	const t = useT();
	const locale = useLocale();
	const localized =
		failure.code !== undefined
			? localizeError(
					t,
					{
						code: failure.code,
						message: failure.message,
						hint: failure.hint,
						details: failure.details,
					},
					{ locale },
				)
			: null;
	const message = localized?.message ?? failure.message;
	const hint = localized !== null ? localized.hint : failure.hint;
	const lang = localized?.localized ? undefined : "en";
	return (
		<div className="flex gap-2 text-sm">
			<CircleAlert
				aria-hidden="true"
				className="mt-0.5 size-4 shrink-0 text-destructive"
			/>
			<div className="min-w-0">
				<p lang={lang}>
					<UntrustedText>{message}</UntrustedText>
				</p>
				{hint !== undefined && (
					<p lang={lang} className="text-muted-foreground">
						{t("ui.editor.palette.how_to_fix")}{" "}
						<UntrustedText>{hint}</UntrustedText>
					</p>
				)}
				{failure.code !== undefined && (
					<p className="font-mono text-xs text-muted-foreground">
						{failure.code}
					</p>
				)}
			</div>
		</div>
	);
}
