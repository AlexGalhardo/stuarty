import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useDocumentState } from "../state/EditorContext";
import ExportDialog from "./ExportDialog";
import JobsChip from "./JobsChip";

// The top-bar entry to the export: the jobs chip, the Export button and the
// dialog both open.
export default function ExportButton(): ReactNode {
	const t = useT();
	const [open, setOpen] = useState(false);
	const ready = useDocumentState(
		(s) => s.status === "ready" || s.status === "offline",
	);
	return (
		<>
			<JobsChip onOpen={() => setOpen(true)} />
			<Button
				type="button"
				size="sm"
				disabled={!ready}
				onClick={() => setOpen(true)}
			>
				{t("ui.editor.export.open")}
			</Button>
			<ExportDialog open={open} onOpenChange={setOpen} />
		</>
	);
}
