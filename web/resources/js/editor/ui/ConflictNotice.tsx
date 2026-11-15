import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";

// The notice of an E_REVISION_CONFLICT (SDD-012 section 4.5, SDD-006 section
// 7.3): who changed what while the user held it and the text the core made of
// the changes. It blocks nothing; the action is drawn by the notice list, which
// also resolves the actor label and the item name from its stores.
export default function ConflictNotice({
	actor,
	item,
	revision,
	diff,
}: {
	actor: string;
	item: string;
	revision: number;
	diff: string;
}): ReactNode {
	const t = useT();
	return (
		<>
			<p className="font-medium">
				{t("ui.editor.conflict.message", { actor, item, revision })}
			</p>
			{diff !== "" && (
				<pre
					lang="en"
					className="mt-1 max-h-32 overflow-auto font-mono text-xs whitespace-pre-wrap text-muted-foreground"
				>
					<UntrustedText>{diff}</UntrustedText>
				</pre>
			)}
		</>
	);
}
