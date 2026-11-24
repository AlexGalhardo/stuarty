import {
	CircleAlert,
	CircleCheck,
	Info,
	type LucideIcon,
	TriangleAlert,
} from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

export type NoticeTone = "info" | "success" | "warning" | "danger";

const LOOK: Record<NoticeTone, { icon: LucideIcon; tone: string }> = {
	info: { icon: Info, tone: "bg-accent-soft text-accent-text" },
	success: { icon: CircleCheck, tone: "bg-success-soft text-success-text" },
	warning: { icon: TriangleAlert, tone: "bg-warning-soft text-warning-text" },
	danger: { icon: CircleAlert, tone: "bg-danger-soft text-danger-text" },
};

// A block that tells the person something that stays on the page: an icon, a
// title, a sentence and optional actions. The tone is told by the icon and by
// the words, never by colour alone. It holds no copy of its own and no
// behaviour (design-system section 4.4); `role` is the caller's, because a
// notice that is always there is a `note` and one that appears is a `status`
// or an `alert`.
export function Notice({
	tone,
	title,
	children,
	actions,
	role = "note",
	className,
	...rest
}: {
	tone: NoticeTone;
	// A node, so a title that holds text a caller wrote can isolate it (UntrustedText).
	title: ReactNode;
	children?: ReactNode;
	actions?: ReactNode;
	role?: "note" | "status" | "alert";
	className?: string;
	"data-testid"?: string;
}): ReactNode {
	const { icon: Icon, tone: colours } = LOOK[tone];
	return (
		<div
			data-slot="notice"
			data-tone={tone}
			role={role}
			className={cn(
				"flex items-start gap-3 rounded-lg px-4 py-3",
				colours,
				className,
			)}
			{...rest}
		>
			<Icon aria-hidden="true" className="mt-0.5 size-5 shrink-0" />
			<div className="min-w-0 flex-1">
				<p className="text-sm font-semibold">{title}</p>
				{children ? <div className="text-sm">{children}</div> : null}
			</div>
			{actions ? (
				<div className="flex shrink-0 flex-wrap items-center gap-2">
					{actions}
				</div>
			) : null}
		</div>
	);
}
