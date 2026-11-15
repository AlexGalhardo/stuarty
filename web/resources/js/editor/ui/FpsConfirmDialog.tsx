import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { useT } from "@/lib/i18n";

// One item the core changed while it repaired the project after the new rate
// (`effective.adjusted` of `settings.set_fps`, SDD-002 C.3), as the core sent it.
export interface AdjustedLine {
	id: string;
	label: string;
	field: string;
	reason: string;
	rescaled: string;
	final: string;
}

export type FpsStage = "confirm" | "busy" | "done";

// Changing the frame rate rescales every time in the project, so it asks first
// (SDD-012 section 12.1) and, once the core has answered, lists the items it had
// to repair (RF-001).
export default function FpsConfirmDialog({
	open,
	rate,
	stage,
	adjusted,
	onConfirm,
	onClose,
}: {
	open: boolean;
	rate: string;
	stage: FpsStage;
	adjusted: readonly AdjustedLine[];
	onConfirm(): void;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const done = stage === "done";
	return (
		<Dialog open={open} onOpenChange={(next) => !next && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{t(
							done
								? "ui.editor.project.fps_done_title"
								: "ui.editor.project.fps_confirm_title",
							{ rate },
						)}
					</DialogTitle>
					<DialogDescription>
						{t(
							done
								? "ui.editor.project.fps_done_body"
								: "ui.editor.project.fps_confirm_body",
							{ count: adjusted.length },
						)}
					</DialogDescription>
				</DialogHeader>
				{done && adjusted.length > 0 && (
					<ul
						aria-label={t("ui.editor.project.fps_adjusted")}
						className="flex max-h-64 flex-col gap-1 overflow-y-auto text-sm"
					>
						{adjusted.map((line) => (
							<li key={`${line.id}-${line.field}-${line.reason}`}>
								<UntrustedText>{line.label}</UntrustedText>:{" "}
								{t("ui.editor.project.fps_adjusted_line", {
									field: line.field,
									from: line.rescaled,
									to: line.final,
									reason: t(`ui.editor.project.adjust_reason.${line.reason}`),
								})}
							</li>
						))}
					</ul>
				)}
				<DialogFooter>
					{done ? (
						<Button type="button" onClick={onClose}>
							{t("ui.editor.project.fps_close")}
						</Button>
					) : (
						<>
							<Button
								type="button"
								variant="outline"
								disabled={stage === "busy"}
								onClick={onClose}
							>
								{t("ui.editor.project.fps_cancel")}
							</Button>
							<Button
								type="button"
								disabled={stage === "busy"}
								onClick={onConfirm}
							>
								{t("ui.editor.project.fps_change")}
							</Button>
						</>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
