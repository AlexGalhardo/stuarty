import { type ComponentProps, type ReactNode, useId } from "react";

import { Input } from "@/components/ui/input";
import { useT } from "@/lib/i18n";

// The input of a time (SDD-012 section 12.2), used by the palette form, the
// inspector and the dialogs. It is a text field that shows what it is given
// and reports what was typed; checking the text and appending the unit is
// `normalizeTimeInput` of commands/units.ts, called by the owner of the value
// (the form or the commit rule of the inspector), so this file needs no
// import from another editor folder. A time is never a number (rule T8).
export default function TimeField({
	value,
	onValueChange,
	invalid = false,
	...rest
}: Omit<ComponentProps<typeof Input>, "value" | "onChange" | "type"> & {
	value: string;
	onValueChange(text: string): void;
	// The caller found the text not to be a time: the inline message shows.
	invalid?: boolean;
}): ReactNode {
	const t = useT();
	const messageId = useId();
	return (
		<div className="flex min-w-0 flex-col gap-1">
			<Input
				{...rest}
				type="text"
				inputMode="decimal"
				autoComplete="off"
				spellCheck={false}
				value={value}
				aria-invalid={invalid || undefined}
				aria-describedby={invalid ? messageId : rest["aria-describedby"]}
				onChange={(event) => onValueChange(event.target.value)}
			/>
			{invalid && (
				<p id={messageId} role="alert" className="text-xs text-destructive">
					{t("ui.editor.errors.E_INVALID_TIME")}. {t("ui.editor.time.hint")}
				</p>
			)}
		</div>
	);
}
