import { type ReactNode, useEffect, useRef } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useLocale, useT } from "@/lib/i18n";
import { localizeError } from "@/lib/localizeError";
import { ApiError } from "../editor/api/errors";

// The error of an action of a page: localized through the catalogue when the
// code/reason is known, falling back to the core's English only for unknown codes.
export interface Failure {
	code: string;
	message: string;
	hint?: string;
	// The field the core says is wrong (`made_for_kids`), so the page can mark it.
	path?: string;
	details?: { readonly [key: string]: unknown };
}

export function failureOf(error: unknown): Failure {
	if (error instanceof ApiError) {
		return {
			code: error.code,
			message: error.message,
			hint: error.hint,
			path: error.path,
			details: error.details,
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

// Whether a note that appears is brought into view. Only a note in a dialog is: the dialog has its own scroll and
// the person just pressed a button in it. A note on a page can come from a background read (the models of a
// connection that needs a sign-in), and scrolling the whole page to it moves the person away from what they
// were reading (VV-431); the alert role announces it where it is.
export function revealsFailure(
	note: { closest(selector: string): unknown } | null,
): boolean {
	return note?.closest('[role="dialog"]') != null;
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
	const locale = useLocale();
	const note = useRef<HTMLDivElement>(null);
	// In a long dialog the note can sit below the fold: it comes into view when it appears.
	useEffect(() => {
		if (failure !== null && revealsFailure(note.current))
			note.current?.scrollIntoView?.({ block: "nearest" });
	}, [failure]);
	if (failure === null) return null;
	const text = localizeError(t, failure, { locale });
	const lang = text.localized ? undefined : "en";
	return (
		<div
			ref={note}
			id={id}
			role="alert"
			className="mt-2 text-sm text-destructive"
		>
			<p className="font-medium">{text.headline}</p>
			<p lang={lang}>
				<UntrustedText>{text.message}</UntrustedText>
				{text.hint !== undefined && (
					<>
						{" "}
						<UntrustedText>{text.hint}</UntrustedText>
					</>
				)}
			</p>
		</div>
	);
}
