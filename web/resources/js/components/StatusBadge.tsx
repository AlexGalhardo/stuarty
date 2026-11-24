import {
	Bot,
	CircleAlert,
	CircleCheck,
	CircleDashed,
	Clock,
	Info,
	type LucideIcon,
} from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

export type Status =
	| "neutral"
	| "info"
	| "agent"
	| "waiting"
	| "success"
	| "danger";

const LOOK: Record<Status, { icon: LucideIcon; tone: string }> = {
	neutral: { icon: CircleDashed, tone: "bg-surface-hover text-text-muted" },
	info: { icon: Info, tone: "bg-accent-soft text-accent-text" },
	agent: { icon: Bot, tone: "bg-agent-soft text-agent-text" },
	waiting: { icon: Clock, tone: "bg-warning-soft text-warning-text" },
	success: { icon: CircleCheck, tone: "bg-success-soft text-success-text" },
	danger: { icon: CircleAlert, tone: "bg-danger-soft text-danger-text" },
};

// A status is always an icon and a word, never a colour alone (design-system
// section 7). The word is the caller's, so it is translated; the icon follows
// the status and is hidden from screen readers, which read the word.
export function StatusBadge({
	status,
	children,
	className,
}: {
	status: Status;
	children: string;
	className?: string;
}): ReactNode {
	const { icon: Icon, tone } = LOOK[status];
	return (
		<span
			data-slot="status-badge"
			data-status={status}
			className={cn(
				"inline-flex w-fit items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap",
				tone,
				className,
			)}
		>
			<Icon aria-hidden="true" className="size-3.5 shrink-0" />
			{children}
		</span>
	);
}
