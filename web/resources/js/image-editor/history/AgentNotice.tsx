import type { ReactNode } from "react";
import { useEffect } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { useImage } from "../state/context";

// How long the outline of what another actor changed stays on the canvas.
export const OUTLINE_MS = 5000;

// A polite live region that says when another actor changed the image (SDD-012 section 24.9). It is on the page
// from the start, so a screen reader hears the text when it appears; the outlines on the canvas go with it.
export default function AgentNotice(): ReactNode {
	const t = useT();
	const notice = useImage((s) => s.notice);
	const clear = useImage((s) => s.clearNotice);

	useEffect(() => {
		if (notice === null) return;
		const timer = window.setTimeout(clear, OUTLINE_MS);
		return () => window.clearTimeout(timer);
	}, [notice, clear]);

	return (
		<div
			role="status"
			aria-live="polite"
			data-testid="image-agent-notice"
			className={
				notice === null ? "sr-only" : "border-b bg-agent/10 px-3 py-1 text-sm"
			}
		>
			{notice !== null && (
				<>
					{t("ui.image.history.agent_changed_prefix")}{" "}
					<UntrustedText>{notice.actor}</UntrustedText>{" "}
					{t("ui.image.history.agent_changed_suffix")}
				</>
			)}
		</div>
	);
}
