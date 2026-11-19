import { X } from "lucide-react";
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
			className="flex items-start justify-between gap-2 border-b bg-destructive/10 px-3 py-1"
		>
			<FailureNote
				failure={{
					code: error.code,
					message: error.message,
					hint: error.hint,
				}}
			/>
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
