import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { ApiError } from "../editor/api/errors";
import { errorHeadline } from "../editor/ui/errorHeadline";

// The error of an action of a page as it came, so the person sees the message
// and the hint of the core; the headline is localized.
export interface Failure {
	code: string;
	message: string;
	hint?: string;
}

export function failureOf(error: unknown): Failure {
	if (error instanceof ApiError) {
		return { code: error.code, message: error.message, hint: error.hint };
	}
	throw error;
}

// `id` lets the fields of the form that failed point at the note with aria-describedby (SDD-012 section 18.10).
export function FailureNote({
	failure,
	id,
}: {
	failure: Failure | null;
	id?: string;
}): ReactNode {
	const t = useT();
	if (failure === null) return null;
	return (
		<div id={id} role="alert" className="mt-2 text-sm text-destructive">
			<p className="font-medium">{errorHeadline(t, failure.code)}</p>
			<p lang="en">
				<UntrustedText>{failure.message}</UntrustedText>
				{failure.hint !== undefined && (
					<>
						{" "}
						<UntrustedText>{failure.hint}</UntrustedText>
					</>
				)}
			</p>
		</div>
	);
}
