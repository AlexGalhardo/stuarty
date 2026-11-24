import { CircleAlert, X } from "lucide-react";
import type { ReactNode } from "react";

import { FailureNote } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useImage } from "./state/context";

// The last error of a command, as the core worded it (SDD-012 section 4.4), until it is dismissed or the next
// command starts.
export default function ErrorBar(): ReactNode {
	const t = useT();
	const error = useImage((s) => s.error);
	const clear = useImage((s) => s.clearError);
	if (error === null) return null;

	return (
		<div
			data-testid="image-error"
			className="flex items-start gap-2 border-b border-border bg-destructive/10 px-3 py-1"
		>
			<CircleAlert
				aria-hidden="true"
				className="mt-2.5 size-4 shrink-0 text-destructive"
			/>
			<div className="min-w-0 flex-1">
				<FailureNote
					failure={{
						code: error.code,
						message: error.message,
						hint: error.hint,
					}}
				/>
			</div>
			<Button
				type="button"
				variant="ghost"
				size="icon-sm"
				aria-label={t("ui.image.error.dismiss")}
				onClick={clear}
			>
				<X aria-hidden="true" />
			</Button>
		</div>
	);
}
