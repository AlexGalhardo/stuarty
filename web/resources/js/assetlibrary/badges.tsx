import { Bot, Box, CircleCheck, Tag, Upload, WandSparkles } from "lucide-react";
import type { ComponentType, ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import type { OriginKind } from "../editor/api/types";

const ORIGIN_ICON: Record<OriginKind, ComponentType<{ className?: string }>> = {
	imported: Upload,
	generated: WandSparkles,
	fetched_by_agent: Bot,
	rendered_externally: Box,
};

const CHIP =
	"inline-flex w-fit items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap";

// Where a file came from, as an icon and a word: never a colour alone (design-system section 7). A file with
// no library row reads as imported, as the service lists it.
export function OriginBadge({ kind }: { kind?: OriginKind }): ReactNode {
	const t = useT();
	const origin = kind ?? "imported";
	const Icon = ORIGIN_ICON[origin];
	return (
		<span
			data-testid="asset-origin-badge"
			className={cn(CHIP, "bg-surface-hover text-text-muted")}
		>
			<Icon aria-hidden="true" className="size-3.5 shrink-0" />
			{t(`ui.assets.origin.${origin}`)}
		</span>
	);
}

// The mark a person set on a file: it may be offered again. Shown only when the service says so.
export function ReusableBadge(): ReactNode {
	const t = useT();
	return (
		<span className={cn(CHIP, "bg-accent-soft text-accent-text")}>
			<CircleCheck aria-hidden="true" className="size-3.5 shrink-0" />
			{t("ui.assets.reusable_badge")}
		</span>
	);
}

// A tag is the person's or an agent's text, so it is drawn as text.
export function TagChip({ tag }: { tag: string }): ReactNode {
	return (
		<span className={cn(CHIP, "bg-surface-hover text-text")}>
			<Tag aria-hidden="true" className="size-3 shrink-0" />
			<UntrustedText>{tag}</UntrustedText>
		</span>
	);
}
