import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";

// Nothing selected: the project panel. A stub that its own task fills (name,
// size, background, loudness, frame rate, template info).
export default function ProjectPanel(): ReactNode {
	const t = useT();
	return (
		<p className="text-sm text-muted-foreground">
			{t("ui.editor.inspector.project_hint")}
		</p>
	);
}
