import {
	CheckCircle2,
	Clock,
	FileText,
	Hourglass,
	Send,
	XCircle,
} from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { useT } from "@/lib/i18n";
import type { TargetState } from "./types";

const ICONS = {
	draft: FileText,
	waiting_approval: Hourglass,
	approved: Clock,
	scheduled: Clock,
	publishing: Send,
	published: CheckCircle2,
	failed: XCircle,
	missed: XCircle,
	cancelled: XCircle,
} as const;

const VARIANTS = {
	draft: "outline",
	waiting_approval: "secondary",
	approved: "default",
	scheduled: "default",
	publishing: "secondary",
	published: "default",
	failed: "destructive",
	missed: "destructive",
	cancelled: "outline",
} as const;

// The state of a target, always in words: the icon and the colour only help.
export function TargetStateBadge({ state }: { state: TargetState }): ReactNode {
	const t = useT();
	const Icon = ICONS[state];
	return (
		<Badge
			variant={VARIANTS[state]}
			data-testid="target-state"
			data-state={state}
		>
			<Icon aria-hidden="true" />
			{t(`ui.publish.state.${state}`)}
		</Badge>
	);
}
