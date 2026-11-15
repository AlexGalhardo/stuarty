import { router } from "@inertiajs/react";

import LocaleSelect from "@/components/LocaleSelect";

// LocaleSwitcher posts the choice to /locale, which stores it in the lang
// cookie; preserveState keeps a half-filled brief while the page reloads.
export default function LocaleSwitcher({
	className = "",
}: {
	className?: string;
}) {
	return (
		<LocaleSelect
			className={className}
			onChange={(next) =>
				router.post(
					"/locale",
					{ locale: next },
					{ preserveState: true, preserveScroll: true },
				)
			}
		/>
	);
}
