import { ChevronDown, ChevronUp, type LucideIcon } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import { useT } from "@/lib/i18n";

// The line that names the tool a panel sends ("Tool recording.set_cursor"): the person and the agent read the
// same name, so what a control does is never a guess (SDD-012 section 12). The name is the command of the
// registry, not a translation.
export function ToolLine({ name }: { name: string }): ReactNode {
	const t = useT();
	return (
		<p className="flex items-baseline gap-2 text-xs text-muted-foreground">
			<span>{t("ui.editor.tool")}</span>
			<code className="font-mono text-foreground" data-tool={name}>
				{name}
			</code>
		</p>
	);
}

// A section of the inspector (docs/design/alpha-editor.md section 3.1): a
// header button with aria-expanded and the fields under it. The header carries
// an optional icon and the chevron at the right edge, as the final design draws it.
export default function Section({
	title,
	name,
	icon: Icon,
	defaultOpen = true,
	children,
}: {
	title: string;
	// Names the section for the keys that open it (`data-section`).
	name?: string;
	icon?: LucideIcon;
	defaultOpen?: boolean;
	children: ReactNode;
}): ReactNode {
	const [open, setOpen] = useState(defaultOpen);
	const bodyId = useId();
	const Chevron = open ? ChevronUp : ChevronDown;
	return (
		<section
			data-section={name}
			className="flex flex-col gap-2 border-b border-border pb-3 last:border-b-0 last:pb-0"
		>
			<button
				type="button"
				aria-expanded={open}
				aria-controls={bodyId}
				className="flex h-8 w-full items-center gap-2 rounded-sm text-left text-sm font-semibold hover:bg-surface-hover"
				onClick={() => setOpen((was) => !was)}
			>
				{Icon !== undefined && (
					<Icon
						aria-hidden="true"
						className="size-4 shrink-0 text-muted-foreground"
					/>
				)}
				<span className="min-w-0 flex-1 truncate">{title}</span>
				<Chevron
					aria-hidden="true"
					className="size-4 shrink-0 text-muted-foreground"
				/>
			</button>
			<div id={bodyId} hidden={!open} className="flex flex-col gap-2">
				{children}
			</div>
		</section>
	);
}
