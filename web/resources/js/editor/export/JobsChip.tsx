import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useStore } from "../state/createStore";
import { useRuntime } from "../state/EditorContext";

// The running jobs with the highest progress (SDD-012 section 14.2). With none
// running it renders nothing; a click opens the export dialog.
export default function JobsChip({ onOpen }: { onOpen(): void }): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const jobs = useStore(runtime.jobs, (s) => s.jobs);
	const running = Object.values(jobs).filter(
		(job) => job.state === "running" || job.state === "queued",
	);
	if (running.length === 0) return null;
	const best = Math.max(...running.map((job) => job.progress));
	return (
		<Button type="button" variant="ghost" size="sm" onClick={onOpen}>
			<span className="[font-variant-numeric:tabular-nums]">
				{t("ui.editor.jobs.chip", {
					count: running.length,
					percent: Math.round(best * 100),
				})}
			</span>
		</Button>
	);
}
