import { type ReactNode, useEffect, useRef } from "react";

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
	// The field the core says is wrong (`made_for_kids`), so the page can mark it.
	path?: string;
}

export function failureOf(error: unknown): Failure {
	if (error instanceof ApiError) {
		return {
			code: error.code,
			message: error.message,
			hint: error.hint,
			path: error.path,
		};
	}
	throw error;
}

// Whether the core says this field is the wrong one (its path is the field, or ends with it).
export function failureIsAbout(
	failure: Failure | null,
	field: string,
): boolean {
	const path = failure?.path;
	return path === field || (path?.endsWith(`.${field}`) ?? false);
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
	const note = useRef<HTMLDivElement>(null);
	// In a long dialog the note can sit below the fold: it comes into view when it appears.
	useEffect(() => {
		if (failure !== null) note.current?.scrollIntoView?.({ block: "nearest" });
	}, [failure]);
	if (failure === null) return null;
	return (
		<div
			ref={note}
			id={id}
			role="alert"
			className="mt-2 text-sm text-destructive"
		>
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
