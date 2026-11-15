import { type ReactNode, useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useStore } from "../state/createStore";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import ExportDialog from "./ExportDialog";
import JobsChip from "./JobsChip";

// The top-bar entry to the export: the jobs chip, the Export button and the
// dialog both open. After a reload `listJobs` hands back an export that is still
// running; the dialog opens once at Running for it (SDD-012 section 14.2).
export default function ExportButton(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const [open, setOpen] = useState(false);
	const exportJobId = useStore(runtime.jobs, (j) => j.exportJobId);
	const rehydrated = useRef(false);
	useEffect(() => {
		if (exportJobId === null || rehydrated.current) return;
		rehydrated.current = true;
		setOpen(true);
	}, [exportJobId]);
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
