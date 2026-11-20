import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { announcedPercent } from "./exportPhase";

// The figure under the bar of a running export (SDD-012 section 18.1 item 5). The exact percent and the
// message of the job change at every progress event, so they are hidden from a screen reader; the polite
// region around them reads a second text that changes only when the progress crosses a step of 10%.
export default function ProgressText({
	progress,
	message,
}: {
	progress: number;
	message?: string;
}): ReactNode {
	const percent = Math.round(progress * 100);
	return (
		<>
			<p
				aria-hidden="true"
				className="text-xs [font-variant-numeric:tabular-nums]"
			>
				{percent}%
				{message !== undefined && (
					<span lang="en">
						{" · "}
						<UntrustedText>{message}</UntrustedText>
					</span>
				)}
			</p>
			<p
				className="sr-only"
				data-announce={announcedPercent(progress)}
				data-testid="export-announced"
			>
				{announcedPercent(progress)}%
			</p>
		</>
	);
}
