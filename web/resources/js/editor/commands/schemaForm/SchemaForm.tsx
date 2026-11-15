import { type ReactNode, useId, useMemo, useState } from "react";

import { useLocale, useT } from "@/lib/i18n";
import type { ApiError } from "../../api/errors";
import type { CommandDetail } from "../../api/types";
import { useDocumentState } from "../../state/EditorContext";
import { readTimeUnit } from "../../state/prefs";
import type { FormSlot, FormSlotProps } from "../palette/formSlot";
import { classify, type IdKind, type Schema } from "./classify";
import { FormProvider, type IdOption } from "./context";
import { type Draft, finalize, toDraft } from "./draft";
import { FormNode } from "./Node";

const NO_OPTIONS: readonly IdOption[] = [];

// The objects an id argument may name, from the read model (label: name and
// id). Free text stays possible: the core checks the id.
function useIdOptions(): Record<IdKind, readonly IdOption[]> {
	const document = useDocumentState((s) => s.document);
	return useMemo(() => {
		if (document === null)
			return { asset: NO_OPTIONS, track: NO_OPTIONS, item: NO_OPTIONS };
		const label = (id: string, name: string | undefined): string =>
			name === undefined || name === "" ? id : `${name} · ${id}`;
		return {
			asset: Object.values(document.assets).map((a) => ({
				id: a.id,
				label: label(a.id, a.name),
			})),
			track: Object.values(document.tracks).map((tr) => ({
				id: tr.id,
				label: label(tr.id, tr.name),
			})),
			item: Object.values(document.items).map((it) => ({
				id: it.id,
				label: label(it.id, it.name),
			})),
		};
	}, [document]);
}

// Where an error `path` (`args.words[0].start`) lands in a schema: the form
// shows it on the field when every step exists there.
export function resolvesPath(schema: Schema, path: string): boolean {
	if (!path.startsWith("args")) return false;
	const steps = path
		.slice("args".length)
		.match(/\.[^.[\]]+|\[\d+\]/g)
		?.filter(Boolean);
	if (steps === undefined) return path === "args";
	let current: Schema = schema;
	for (const step of steps) {
		if (step.startsWith("[")) {
			const construct = classify(current, "");
			if (construct.kind !== "list") return false;
			current = construct.items;
		} else {
			const properties = current.properties;
			const next =
				typeof properties === "object" && properties !== null
					? (properties as { [key: string]: unknown })[step.slice(1)]
					: undefined;
			if (typeof next !== "object" || next === null) return false;
			current = next as Schema;
		}
	}
	return true;
}

function Body({
	command,
	value,
	onChange,
	error,
}: Omit<FormSlotProps, "revision">): ReactNode {
	const t = useT();
	const locale = useLocale();
	const options = useIdOptions();
	const prefix = useId();
	const schema = command.args_schema as Schema;
	const [draft, setDraft] = useState<Draft>(() => toDraft(schema, "", value));
	const root = classify(schema, "");

	const change = (next: Draft): void => {
		setDraft(next);
		const result = finalize(schema, next, { unit: readTimeUnit(), locale });
		onChange(result.ok ? result.value : null);
	};

	if (root.kind !== "object") {
		return (
			<p className="text-sm text-muted-foreground">
				{t("ui.editor.palette.no_arguments")}
			</p>
		);
	}
	return (
		<FormProvider value={{ locale, options, error, prefix }}>
			<div className="flex flex-col gap-3" data-form={command.name}>
				<FormNode
					schema={schema}
					name=""
					path="args"
					required
					draft={draft}
					inline
					onChange={change}
				/>
			</div>
		</FormProvider>
	);
}

// The form of SDD-012 section 9.2, generated from `args_schema`. The palette
// owns the value; the form keeps a draft (the text of a time field as typed)
// and hands the palette the arguments, or null while a time or a JSON
// fallback is not valid, so Run never sends a bare number as a time (T8).
export const SchemaForm: FormSlot = ({ revision, ...props }) => (
	<Body key={revision} {...props} />
);

SchemaForm.resolvesPath = (command: CommandDetail, error: ApiError): boolean =>
	error.path !== undefined &&
	resolvesPath(command.args_schema as Schema, error.path);
