import { createContext, useContext } from "react";

import type { ApiError } from "../../api/errors";
import type { IdKind } from "./classify";

export interface IdOption {
	id: string;
	label: string;
}

// What every field of one form shares: the objects an id may name (from the
// read model), the locale for the decimal comma and the error of the last send.
export interface FormContext {
	locale: string;
	options: Readonly<Record<IdKind, readonly IdOption[]>>;
	error: ApiError | null;
	// A prefix that keeps the DOM ids of two forms apart.
	prefix: string;
}

const Context = createContext<FormContext | null>(null);

export const FormProvider = Context.Provider;

export function useForm(): FormContext {
	const value = useContext(Context);
	if (value === null) throw new Error("FormProvider is missing");
	return value;
}
