import type { ReactNode } from "react";

import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { useT } from "@/lib/i18n";
import type { ApiClient } from "../api/client";
import RecordPanel from "./RecordPanel";
import RecordTeleprompter from "./RecordTeleprompter";
import { useRecordFlow } from "./useRecordFlow";

// The record dialog (SDD-025 section 12): the source picker, the visible state of the recording with its stop
// button, and the step where a person adds the recording to the project or says "Not now". It computes nothing.
export default function RecordDialog({
	client,
	projectId,
	open,
	onOpenChange,
}: {
	client: ApiClient;
	projectId: string;
	open: boolean;
	onOpenChange: (open: boolean) => void;
}): ReactNode {
	const t = useT();
	const flow = useRecordFlow(client, projectId, open);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t("ui.editor.record.title")}</DialogTitle>
					<DialogDescription>
						{t("ui.editor.record.description")}
					</DialogDescription>
				</DialogHeader>
				<RecordPanel flow={flow} onClose={() => onOpenChange(false)} />
				<RecordTeleprompter projectId={projectId} />
			</DialogContent>
		</Dialog>
	);
}
