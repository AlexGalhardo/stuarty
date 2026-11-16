import { type ReactNode, useId } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { useActorLabel } from "../../state/useActorLabel";
import { errorHeadline } from "../../ui/errorHeadline";
import type { FieldSnapshot } from "./commitField";
import { rangeOf, withoutArgPath } from "./fieldMessage";

// The message line under a field (docs/design/alpha-editor.md section 3.6): the
// error as received, or the headline and the hint of a time the browser
// refused to send, and the note that another actor changed the value.
export function FieldMessage({
	id,
	state,
}: {
	id: string;
	state: Pick<FieldSnapshot, "error" | "remoteActor">;
}): ReactNode {
	const t = useT();
	const actorLabel = useActorLabel();
	const { error, remoteActor } = state;
	if (error === null && remoteActor === null) return null;
	return (
		<div id={id} className="flex flex-col gap-0.5 text-xs">
			{error !== null &&
				(error.message === "" ? (
					<p role="alert" className="text-destructive">
						{errorHeadline(t, error.code)}.{" "}
						{rangeOf(error.details) !== null
							? t("ui.editor.palette.range", { ...rangeOf(error.details) })
							: t(
									error.code === "E_INVALID_TIME"
										? "ui.editor.time.hint"
										: "ui.editor.inspector.invalid_value",
								)}
					</p>
				) : (
					<p role="alert" lang="en" className="text-destructive">
						<UntrustedText>{withoutArgPath(error.message)}</UntrustedText>
						{error.hint !== undefined && (
							<>
								{" "}
								<UntrustedText>{error.hint}</UntrustedText>
							</>
						)}
					</p>
				))}
			{remoteActor !== null && (
				<p role="status" className="text-agent">
					{t("ui.editor.inspector.changed_by", {
						actor: actorLabel(remoteActor),
					})}
				</p>
			)}
		</div>
	);
}

// A row of the inspector: a label and the control, 96 px and the rest.
export function FieldRow({
	label,
	state,
	children,
}: {
	label: string;
	state: Pick<FieldSnapshot, "error" | "remoteActor">;
	children: (ids: { control: string; message: string }) => ReactNode;
}): ReactNode {
	const base = useId();
	const ids = { control: `${base}-control`, message: `${base}-message` };
	return (
		<div className="grid grid-cols-[96px_1fr] items-start gap-x-2 gap-y-1">
			<Label htmlFor={ids.control} className="pt-1.5 text-xs">
				{label}
			</Label>
			<div className="flex min-w-0 flex-col gap-1">
				{children(ids)}
				<FieldMessage id={ids.message} state={state} />
			</div>
		</div>
	);
}
