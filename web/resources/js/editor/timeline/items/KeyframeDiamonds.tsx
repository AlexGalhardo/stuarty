import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { KEYFRAME_PROPERTIES } from "../../commands/builders";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../../state/EditorContext";

// One diamond per keyframe of the item, from the `keyframes` keys of the read
// model (SDD-012 section 7.4): display only, placed by the zoom variable of
// the timeline like the item itself. A click selects the keyframe, which
// Delete then removes; no diamond moves or computes anything.
export default function KeyframeDiamonds({ id }: { id: string }): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const keyframes = useDocumentState((s) => s.document?.items[id]?.keyframes);
	const picked = useUiState((s) =>
		s.selection.keyframe?.item === id ? s.selection.keyframe : null,
	);
	if (keyframes === undefined) return null;
	const names = KEYFRAME_PROPERTIES.filter(
		(name) => (keyframes[name]?.length ?? 0) > 0,
	);
	return (
		<>
			{names.flatMap((name, lane) =>
				(keyframes[name] ?? []).map((key) => {
					const isPicked = picked?.property === name && picked.t === key.t;
					return (
						<button
							key={`${name}:${key.t}`}
							type="button"
							tabIndex={-1}
							data-keyframe-property={name}
							data-keyframe-t={key.t}
							data-selected={isPicked ? "" : undefined}
							aria-label={t("ui.editor.keyframes.diamond", {
								property: t(`ui.editor.keyframes.prop.${name}`),
							})}
							aria-pressed={isPicked}
							className={cn(
								"absolute z-10 size-2.5 -translate-x-1/2 rotate-45 cursor-pointer border border-foreground bg-background",
								isPicked && "bg-ed-selection",
							)}
							style={{
								["--t" as string]: key.t,
								["--lane" as string]: lane,
								left: "calc(var(--t) * var(--ppf) * 1px)",
								bottom: "calc(2px + var(--lane) * 9px)",
							}}
							onPointerDown={(event) => event.stopPropagation()}
							onDoubleClick={(event) => event.stopPropagation()}
							onClick={(event) => {
								event.stopPropagation();
								runtime.uiWriter.selectKeyframe({
									item: id,
									property: name,
									t: key.t,
								});
							}}
						/>
					);
				}),
			)}
		</>
	);
}
