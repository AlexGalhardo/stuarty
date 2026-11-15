import { CircleAlert } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";

// What the editor shows for an error: the code, the message and the hint of the
// core when it is a core error (SDD-012 sections 4.4 and 5.6), never a blank page.
export interface Failure {
	code?: string;
	message: string;
	hint?: string;
}

export function FailureView({ failure }: { failure: Failure }): ReactNode {
	const t = useT();
	return (
		<div className="flex gap-2 text-sm">
			<CircleAlert
				aria-hidden="true"
				className="mt-0.5 size-4 shrink-0 text-destructive"
			/>
			<div className="min-w-0">
				<p lang="en">
					<UntrustedText>{failure.message}</UntrustedText>
				</p>
				{failure.hint !== undefined && (
					<p lang="en" className="text-muted-foreground">
						{t("ui.editor.palette.how_to_fix")}{" "}
						<UntrustedText>{failure.hint}</UntrustedText>
					</p>
				)}
				{failure.code !== undefined && (
					<p className="font-mono text-xs text-muted-foreground">
						{failure.code}
					</p>
				)}
			</div>
		</div>
	);
}
