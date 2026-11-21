import { ApiError } from "../editor/api/errors";
import type { AccountFieldProblem } from "../editor/api/types";

// The refused fields of an account form, from the `details.fields` the server answers an E_VALIDATION with
// (SDD-007 section 3.9 item 1). The rules are the server's: this only reads what it said.
export function fieldProblems(error: unknown): AccountFieldProblem[] {
	if (!(error instanceof ApiError) || error.code !== "E_VALIDATION") return [];
	const fields = error.details?.fields;
	if (!Array.isArray(fields)) return [];
	const out: AccountFieldProblem[] = [];
	for (const entry of fields) {
		if (
			typeof entry === "object" &&
			entry !== null &&
			!Array.isArray(entry) &&
			typeof entry.field === "string" &&
			typeof entry.reason === "string"
		) {
			out.push({ field: entry.field, reason: entry.reason });
		}
	}
	return out;
}

// The reasons of one field.
export function reasonsOf(
	problems: readonly AccountFieldProblem[],
	field: string,
): string[] {
	return problems.filter((p) => p.field === field).map((p) => p.reason);
}
