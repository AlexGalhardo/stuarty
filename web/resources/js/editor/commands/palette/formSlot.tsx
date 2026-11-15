import { type ReactNode, useState } from "react";

import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import type { ApiError } from "../../api/errors";
import type { CommandDetail, JsonValue } from "../../api/types";

export type CommandArgs = { [key: string]: JsonValue };

// The slot of the palette form (SDD-012 section 9.2). The palette owns the
// value; the slot edits it. `onChange(null)` means the text is not a valid
// value yet, which disables Preview, Run and Add to batch. P2-UI-004 renders
// this slot from `args_schema`; until then it is a JSON textarea.
export interface FormSlotProps {
	command: CommandDetail;
	value: CommandArgs;
	// Changes when the palette replaced the value from outside (Fill example).
	revision: number;
	onChange(value: CommandArgs | null): void;
	// The error of the last send, for field-level display.
	error: ApiError | null;
}

export type FormSlot = ((props: FormSlotProps) => ReactNode) & {
	// Whether the form shows this error on one of its fields; when it does not,
	// the palette shows the error block at the top of the form.
	resolvesPath?(command: CommandDetail, error: ApiError): boolean;
};

function parse(text: string): CommandArgs | null {
	try {
		const value: unknown = JSON.parse(text);
		if (typeof value === "object" && value !== null && !Array.isArray(value))
			return value as CommandArgs;
	} catch {
		// Not valid JSON yet.
	}
	return null;
}

export const JsonArgsForm: FormSlot = ({ value, revision, onChange }) => (
	<JsonTextarea key={revision} initial={value} onChange={onChange} />
);

function JsonTextarea({
	initial,
	onChange,
}: {
	initial: CommandArgs;
	onChange(value: CommandArgs | null): void;
}): ReactNode {
	const t = useT();
	const [text, setText] = useState(() => JSON.stringify(initial, null, 2));
	const invalid = parse(text) === null;
	return (
		<div className="flex flex-col gap-1.5">
			<Label htmlFor="palette-args">{t("ui.editor.palette.args_json")}</Label>
			<Textarea
				id="palette-args"
				className="min-h-32 font-mono text-xs"
				spellCheck={false}
				aria-invalid={invalid}
				value={text}
				onChange={(event) => {
					setText(event.target.value);
					onChange(parse(event.target.value));
				}}
			/>
			{invalid && (
				<p role="alert" className="text-xs text-destructive">
					{t("ui.editor.palette.invalid_json")}
				</p>
			)}
		</div>
	);
}
