import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { ErrorState } from "./ErrorState";
import type { Failure } from "./Failure";

type Kind = "network" | "locked" | "generic";

function kindOf(code: string): Kind {
	if (code === "E_NETWORK") return "network";
	if (code === "E_LOCKED" || code === "E_UNAUTHORIZED") return "locked";
	return "generic";
}

const STEPS: Record<Kind, number> = { network: 3, locked: 2, generic: 2 };

// The error of a read that fills a page, in a person's words: what happened, "How to fix" and "Try again". The
// raw message of the server is not shown (it is English and technical); the code only chooses which words.
// A page that wants its own words uses ErrorState directly.
export function LoadError({
	failure,
	onRetry,
	className,
}: {
	failure: Failure;
	onRetry?: () => void;
	className?: string;
}): ReactNode {
	const t = useT();
	const kind = kindOf(failure.code);
	const steps = Array.from({ length: STEPS[kind] }, (_, i) =>
		t(`ui.state.error.${kind}.step${i + 1}`),
	);
	return (
		<ErrorState
			title={t(`ui.state.error.${kind}.title`)}
			description={t(`ui.state.error.${kind}.body`)}
			steps={steps}
			onRetry={onRetry}
			className={className}
		/>
	);
}
