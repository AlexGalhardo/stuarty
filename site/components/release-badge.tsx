import { TriangleAlert } from "lucide-react";
import { ui as t } from "@/lib/ui-text";

/**
 * The release stage and the version, right of the logo. The version is the latest release read when the site is
 * built (lib/releases.ts), passed down by the layout; the badge never holds a literal. Stacked under the logo on a
 * phone so the controls of the bar keep their 44 px targets.
 */
export function ReleaseBadge({ version }: { version: string }) {
	return (
		<span className="flex shrink-0 items-center gap-1.5 whitespace-nowrap" data-release-badge>
			<span className="inline-flex h-5 items-center gap-1 rounded-full bg-warning-soft px-2 text-xs leading-none font-semibold text-warning-text">
				<TriangleAlert aria-hidden className="size-3" />
				{t.alpha}
			</span>
			<span className="font-mono text-xs leading-none text-faint">v{version}</span>
		</span>
	);
}
