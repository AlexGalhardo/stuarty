import { Languages } from "lucide-react";

import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import { useLocale, useLocales, useT } from "@/lib/i18n";

// The language select of every page and of the editor. It only reports the
// choice: an Inertia page posts it with the router, the editor posts it and
// reloads at the same playhead (SDD-012 section 16.3).
export default function LocaleSelect({
	onChange,
	className = "",
}: {
	onChange(locale: string): void;
	className?: string;
}) {
	const t = useT();
	const locale = useLocale();
	const locales = useLocales();

	if (locales.length < 2) return null;

	return (
		<Select value={locale} onValueChange={onChange}>
			<SelectTrigger
				size="sm"
				aria-label={t("ui.nav.language")}
				className={`w-auto gap-2 ${className}`}
			>
				<Languages aria-hidden="true" className="size-4" />
				<SelectValue />
			</SelectTrigger>
			<SelectContent position="popper" align="end">
				{locales.map((l) => (
					<SelectItem key={l} value={l}>
						{t(`ui.nav.languages.${l}`)}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	);
}
