import type { ReactNode } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import { LoadingState } from "@/components/LoadingState";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";

// What the editor draws before it has a text: the loading sentence while the service answers, and the failure it
// answered with and a button to read the text again.
export default function EditorState({
	failure,
	onRetry,
}: {
	failure: Failure | null;
	onRetry: () => void;
}): ReactNode {
	const t = useT();
	return (
		<div className="flex flex-col gap-3" data-testid="editor-state">
			<FailureNote failure={failure} />
			{failure === null ? (
				<LoadingState message={t("ui.write.editor.loading")} />
			) : (
				<div>
					<Button type="button" variant="outline" onClick={onRetry}>
						{t("ui.write.editor.retry")}
					</Button>
				</div>
			)}
		</div>
	);
}
