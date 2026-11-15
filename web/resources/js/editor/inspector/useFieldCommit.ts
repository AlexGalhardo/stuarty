import { useCallback } from "react";
import { FIELD_ERRORS } from "../commands/actions";
import type { Operations } from "../commands/builders";
import { useActions } from "../commands/useActions";
import type { CommitResult } from "./fields/commitField";

// The send of an inspector field: one batch through the action registry. An
// error the field shows under itself comes back as a result; every other error
// was already raised as a notice by the action (SDD-012 section 8.4).
export function useFieldCommit(): (ops: Operations) => Promise<CommitResult> {
	const actions = useActions();
	return useCallback(
		async (ops) => {
			const outcome = await actions.commitField(ops);
			if (outcome === null || outcome.ok) return { ok: true };
			const { code, message, hint, details } = outcome.error;
			return FIELD_ERRORS.has(code)
				? { ok: false, error: { code, message, hint, details } }
				: { ok: true };
		},
		[actions],
	);
}
