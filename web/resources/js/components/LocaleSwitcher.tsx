import { router } from "@inertiajs/react";

import LocaleSelect from "@/components/LocaleSelect";
import { writeLocale } from "@/editor/state/prefs";

// LocaleSwitcher keeps the choice in this browser (the Vukem-Video-Lang header
// carries it from then on) and reloads the page; a reload keeps the state of
// the page, so a half-filled brief survives.
export default function LocaleSwitcher({
	className = "",
}: {
	className?: string;
}) {
	return (
		<LocaleSelect
			className={className}
			onChange={(next) => {
				writeLocale(next);
				document.documentElement.lang = next;
				router.reload();
			}}
		/>
	);
}
