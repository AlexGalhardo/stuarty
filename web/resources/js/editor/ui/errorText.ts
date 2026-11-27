import type { Translate } from "@/lib/i18n";
import {
	type ErrorLike,
	type ErrorTextContext,
	localizeError,
} from "@/lib/localizeError";

export type { ErrorLike, ErrorTextContext };

export interface ErrorText {
	message: string;
	hint: string | undefined;
	// False when the text is the server's English as received, so the caller can
	// mark it `lang="en"`.
	localized: boolean;
}

// The catalogue wording of an error, from its code and its `details` (counts,
// frames, names); an unknown code keeps the server's English message and hint
// (VV-126, epic 113). Only the wording changes: every value comes from the core.
export function errorText(
	t: Translate,
	error: ErrorLike,
	ctx: ErrorTextContext,
	locale?: string,
): ErrorText {
	const result = localizeError(t, error, { ctx, locale });
	return {
		message: result.message,
		hint: result.hint,
		localized: result.localized,
	};
}
