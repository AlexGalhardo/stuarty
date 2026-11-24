import { Lightbulb } from "lucide-react";
import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";

// What the person can ask first, as sentences that fill the composer (they send nothing). The sentences follow the
// kind of the project the service reports: a text-only project gets prompts about text and none about clips or
// frames (author decision D-134). A kind that is not known gets the neutral ones.
const SETS: Record<string, readonly string[]> = {
	text: ["outline", "tighten", "headlines"],
	video: ["script", "captions", "hook"],
	image: ["describe", "variants", "alt"],
};
const NEUTRAL = ["summary", "next", "ideas"] as const;

export default function StarterPrompts({
	kind,
	onPick,
}: {
	kind: string | undefined;
	onPick(text: string): void;
}): ReactNode {
	const t = useT();
	const set = (kind !== undefined && SETS[kind]) || NEUTRAL;
	const group = kind !== undefined && kind in SETS ? kind : "neutral";
	return (
		<section
			aria-label={t("ui.chat.starters")}
			data-testid="chat-starters"
			data-kind={group}
			className="flex flex-col gap-2 rounded-lg border border-dashed border-border-strong px-3 py-3"
		>
			<p className="flex items-center gap-1.5 text-xs font-medium text-text-muted">
				<Lightbulb aria-hidden="true" className="size-3.5 shrink-0" />
				{t("ui.chat.starters")}
			</p>
			<ul className="flex flex-col gap-1">
				{set.map((key) => (
					<li key={key}>
						<button
							type="button"
							onClick={() => onPick(t(`ui.chat.starter.${group}.${key}`))}
							className="w-full rounded-md px-2 py-1.5 text-start text-sm text-text hover:bg-surface-hover focus-visible:ring-3 focus-visible:ring-ring/50"
						>
							{t(`ui.chat.starter.${group}.${key}`)}
						</button>
					</li>
				))}
			</ul>
		</section>
	);
}
