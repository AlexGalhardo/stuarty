import type { ReactNode } from "react";

import { type Translate, useT } from "@/lib/i18n";
import { useAnnouncement } from "./announce";

const ERROR_PREFIX = "ui.editor.errors.";

// The headline of an error code, or the generic one when the catalogue has none
// (the same rule as ui/errorHeadline.ts, which this folder may not import).
function headline(t: Translate, key: string): string {
	const text = t(key);
	return text === key ? t(`${ERROR_PREFIX}unknown`) : text;
}

// The visually hidden polite region of SDD-012 section 18.1 item 5. Errors
// announce the headline only; the full message is in the toast.
export default function LiveRegion(): ReactNode {
	const t = useT();
	const announcement = useAnnouncement();
	let text = "";
	if (announcement !== null) {
		text = announcement.key.startsWith(ERROR_PREFIX)
			? headline(t, announcement.key)
			: t(announcement.key, announcement.params);
	}
	return (
		<div
			role="status"
			aria-live="polite"
			aria-atomic="true"
			data-live-region=""
			className="sr-only"
		>
			{text}
		</div>
	);
}
