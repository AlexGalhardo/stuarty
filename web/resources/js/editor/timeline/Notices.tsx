import { X } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import ErrorBody from "../commands/ErrorBody";
import { useActions } from "../commands/useActions";
import { useStore } from "../state/createStore";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import type { Notice } from "../state/noticeStore";
import { selectItem } from "../state/selectors";
import { useActorLabel } from "../state/useActorLabel";
import ConflictNotice from "../ui/ConflictNotice";
import { errorHeadline } from "../ui/errorHeadline";
import { capitalizeFirst } from "../ui/format";

function ConflictBody({
	conflict,
}: {
	conflict: NonNullable<Notice["conflict"]>;
}): ReactNode {
	const t = useT();
	const actorLabel = useActorLabel();
	const item = useDocumentState((s) =>
		conflict.itemId === null ? undefined : selectItem(s, conflict.itemId),
	);
	return (
		<ConflictNotice
			fallbackActor={conflict.actor === ""}
			actor={
				conflict.actor === ""
					? t("ui.editor.notice.another_actor")
					: actorLabel(conflict.actor)
			}
			item={
				item === undefined
					? t("ui.editor.conflict.something")
					: (item.name ?? t(`ui.editor.item.${item.type}`))
			}
			revision={conflict.revision}
			diff={conflict.diff}
		/>
	);
}

// The notices of SDD-012 section 8.4: the headline of the code, then the core's
// message and hint as received, and the one action the core made possible.
// Only the localized fallback actor can start a sentence in lower case; the id
// of a real actor is written as it is.
function lead(text: string, fallback: boolean): string {
	return fallback ? capitalizeFirst(text) : text;
}

// At most this many notices are drawn, the newest ones; the store keeps a few
// more, so dismissing one brings the next back.
export const MAX_NOTICES_SHOWN = 3;

export default function Notices(): ReactNode {
	const t = useT();
	const actorLabel = useActorLabel();
	const runtime = useRuntime();
	const actions = useActions();
	const notices = useStore(runtime.notices, (s) => s.notices);
	if (notices.length === 0) return null;
	return (
		// The stack never takes a pointer from the editor under it (a run of
		// failed edits stacks notices over the timeline); only the buttons do.
		<div className="pointer-events-none fixed right-3 bottom-3 z-50 flex w-96 flex-col gap-2">
			{notices.slice(-MAX_NOTICES_SHOWN).map((notice) => (
				<div
					key={notice.id}
					role={notice.kind === "error" ? "alert" : "status"}
					data-notice-code={notice.code}
					className="flex items-start gap-2 rounded-md border border-ed-track-line bg-ed-panel p-3 text-sm shadow-md"
				>
					<div className="min-w-0 flex-1">
						{notice.conflict !== undefined ? (
							<ConflictBody conflict={notice.conflict} />
						) : (
							<>
								<p className="font-medium">
									{notice.messageKey !== undefined
										? lead(
												t(notice.messageKey, {
													...notice.params,
													count: notice.count ?? 0,
													actor:
														notice.actor === undefined
															? t("ui.editor.notice.another_actor")
															: actorLabel(notice.actor),
												}),
												notice.actor === undefined,
											)
										: notice.code !== undefined
											? errorHeadline(t, notice.code)
											: null}
								</p>
								<ErrorBody
									error={{
										code: notice.code ?? "",
										message: notice.message,
										hint: notice.hint,
										details: notice.details,
									}}
									className="text-xs"
									hintClassName="text-xs text-muted-foreground"
								/>
							</>
						)}
						{notice.action !== undefined && (
							<Button
								type="button"
								size="sm"
								variant="outline"
								className="pointer-events-auto mt-2"
								onClick={() => void actions.applyNotice(notice)}
							>
								{t(notice.action.labelKey)}
							</Button>
						)}
					</div>
					<Button
						type="button"
						size="icon-sm"
						variant="ghost"
						className="pointer-events-auto"
						aria-label={t("ui.editor.notice.dismiss")}
						onClick={() => runtime.noticeWriter.dismiss(notice.id)}
					>
						<X aria-hidden="true" />
					</Button>
				</div>
			))}
		</div>
	);
}
