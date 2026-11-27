import { CircleHelp, Info } from "lucide-react";
import type { ReactNode } from "react";

import { Label } from "@/components/ui/label";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";

// Compact keyboard- and pointer-accessible info trigger backed by Base UI Tooltip
// (design-system section 4.4, epic 112). Keeps a 24x24 CSS px minimum target
// (WCAG 2.5.8) so it works in both dashboard forms and dense editor inspectors
// without adding vertical space.
export function FieldHelpTip({
	help,
	ariaLabel,
	side = "top",
	className,
}: {
	help: ReactNode;
	ariaLabel: string;
	side?: "top" | "right" | "bottom" | "left";
	className?: string;
}): ReactNode {
	return (
		<Tooltip>
			<TooltipTrigger
				type="button"
				data-slot="field-help-trigger"
				className={cn(
					"inline-flex size-6 shrink-0 items-center justify-center rounded-full text-text-muted hover:bg-surface-hover hover:text-text",
					className,
				)}
			>
				<CircleHelp aria-hidden="true" className="size-3.5" />
				<span className="sr-only">{ariaLabel}</span>
			</TooltipTrigger>
			<TooltipContent
				side={side}
				className="max-w-xs text-left leading-relaxed"
			>
				{help}
			</TooltipContent>
		</Tooltip>
	);
}

// Pairs a visible <Label> with an accessible contextual tooltip explaining what
// the field does, why it matters, and what format it expects (epic 112).
export function FieldLabel({
	htmlFor,
	id,
	label,
	labelText,
	help,
	helpAriaLabel,
	side = "top",
	className,
	labelClassName,
}: {
	htmlFor?: string;
	id?: string;
	label: ReactNode;
	// Plain text of `label` when `label` is a composite ReactNode, used to build the tooltip trigger name.
	labelText?: string;
	help: ReactNode;
	helpAriaLabel?: string;
	side?: "top" | "right" | "bottom" | "left";
	className?: string;
	labelClassName?: string;
}): ReactNode {
	const t = useT();
	const fieldName =
		labelText ?? (typeof label === "string" ? label : t("ui.help.field"));
	const triggerLabel =
		helpAriaLabel ?? t("ui.help.more_info", { field: fieldName });

	return (
		<div
			data-slot="field-label"
			className={cn("flex items-center gap-1", className)}
		>
			<Label id={id} htmlFor={htmlFor} className={labelClassName}>
				{label}
			</Label>
			<FieldHelpTip help={help} ariaLabel={triggerLabel} side={side} />
		</div>
	);
}

// Inline supporting description below a field, referenced via `aria-describedby`
// on the matching control so screen readers announce it automatically.
export function FieldHint({
	id,
	children,
	className,
}: {
	id: string;
	children: ReactNode;
	className?: string;
}): ReactNode {
	return (
		<p
			id={id}
			data-slot="field-hint"
			className={cn("text-xs leading-relaxed text-text-subtle", className)}
		>
			{children}
		</p>
	);
}

// Concise explanatory help card at the top of multi-field or advanced forms
// (connections, SMART goals, prompt templates, cost ceilings, webhooks, export
// options, recurring calendar slots). Uses verified semantic contrast pairs on
// `surface-sunken` in both light and dark themes.
export function HelpCard({
	title,
	description,
	examples,
	examplesLabel,
	collapsible = false,
	defaultOpen = false,
	className,
	"data-testid": testId,
}: {
	title: ReactNode;
	description: ReactNode;
	examples?: readonly string[];
	examplesLabel?: string;
	collapsible?: boolean;
	defaultOpen?: boolean;
	className?: string;
	"data-testid"?: string;
}): ReactNode {
	const t = useT();
	const body = (
		<div className="flex flex-col gap-2 text-sm text-text-muted">
			<p className="leading-relaxed">{description}</p>
			{examples !== undefined && examples.length > 0 && (
				<div className="flex flex-wrap items-center gap-1.5 text-xs">
					<span className="font-medium text-text-subtle">
						{examplesLabel ?? t("ui.help.examples")}
					</span>
					{examples.map((example) => (
						<code
							key={example}
							className="rounded-sm border border-border bg-surface px-1.5 py-0.5 font-mono text-xs text-text"
						>
							{example}
						</code>
					))}
				</div>
			)}
		</div>
	);

	if (collapsible) {
		return (
			<details
				open={defaultOpen}
				data-slot="help-card"
				data-testid={testId}
				className={cn(
					"rounded-lg border border-border bg-surface-sunken px-4 py-3 text-text",
					className,
				)}
			>
				<summary className="flex cursor-pointer items-center gap-2 text-sm font-semibold text-accent-text">
					<Info aria-hidden="true" className="size-4 shrink-0" />
					<span>{title}</span>
				</summary>
				<div className="mt-2 pl-6">{body}</div>
			</details>
		);
	}

	return (
		<div
			role="note"
			data-slot="help-card"
			data-testid={testId}
			className={cn(
				"flex items-start gap-3 rounded-lg border border-border bg-surface-sunken px-4 py-3 text-text",
				className,
			)}
		>
			<Info
				aria-hidden="true"
				className="mt-0.5 size-4 shrink-0 text-accent-text"
			/>
			<div className="flex min-w-0 flex-1 flex-col gap-1">
				<p className="text-sm font-semibold text-text">{title}</p>
				{body}
			</div>
		</div>
	);
}
