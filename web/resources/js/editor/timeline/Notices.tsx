import { X } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useActions } from "../commands/useActions";
import { useStore } from "../state/createStore";
import { useRuntime } from "../state/EditorContext";
import { errorHeadline } from "../ui/errorHeadline";

// The notices of SDD-012 section 8.4: the headline of the code, then the core's
// message and hint as received, and the one action the core made possible.
export default function Notices(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const notices = useStore(runtime.notices, (s) => s.notices);
	if (notices.length === 0) return null;
	return (
		<div className="fixed right-3 bottom-3 z-50 flex w-96 flex-col gap-2">
			{notices.map((notice) => (
				<div
					key={notice.id}
					role={notice.kind === "error" ? "alert" : "status"}
					data-notice-code={notice.code}
					className="flex items-start gap-2 rounded-md border border-ed-track-line bg-ed-panel p-3 text-sm shadow-md"
				>
					<div className="min-w-0 flex-1">
						<p className="font-medium">
							{notice.messageKey !== undefined
								? t(notice.messageKey, { count: notice.count ?? 0 })
								: notice.code !== undefined
									? errorHeadline(t, notice.code)
									: null}
						</p>
						{notice.message !== "" && (
							<p lang="en" className="text-xs">
								<UntrustedText>{notice.message}</UntrustedText>
							</p>
						)}
						{notice.hint !== undefined && (
							<p lang="en" className="text-xs text-muted-foreground">
								<UntrustedText>{notice.hint}</UntrustedText>
							</p>
						)}
						{notice.action !== undefined && (
							<Button
								type="button"
								size="sm"
								variant="outline"
								className="mt-2"
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
