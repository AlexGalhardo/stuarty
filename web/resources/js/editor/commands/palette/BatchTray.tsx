import { X } from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { BatchEntry } from "./batch";

// The footer tray of the palette batch (docs/design/alpha-editor.md section
// 2.4, variant A): visible only while the batch has an operation.
export default function BatchTray({
	batch,
	disabled,
	busy,
	onRemove,
	onClear,
	onPreview,
	onRun,
}: {
	batch: readonly BatchEntry[];
	disabled: boolean;
	busy: boolean;
	onRemove(id: number): void;
	onClear(): void;
	onPreview(): void;
	onRun(): void;
}): ReactNode {
	const t = useT();
	if (batch.length === 0) return null;
	return (
		<section
			aria-label={t("ui.editor.palette.batch.label")}
			className="flex flex-wrap items-center gap-2 border-t border-border px-3 py-2"
		>
			<span className="text-xs font-medium">
				{t("ui.editor.palette.batch.count", { count: batch.length })}
			</span>
			<ul className="flex min-w-0 flex-1 flex-wrap gap-1">
				{batch.map((entry) => (
					<li key={entry.id}>
						<Badge variant="secondary" className="gap-1 font-mono">
							{entry.operation.op}
							<button
								type="button"
								className="rounded-sm"
								aria-label={t("ui.editor.palette.batch.remove", {
									name: entry.operation.op,
								})}
								onClick={() => onRemove(entry.id)}
							>
								<X aria-hidden="true" className="size-3" />
							</button>
						</Badge>
					</li>
				))}
			</ul>
			<Button type="button" size="sm" variant="ghost" onClick={onClear}>
				{t("ui.editor.palette.batch.clear")}
			</Button>
			<Button
				type="button"
				size="sm"
				variant="outline"
				disabled={disabled}
				onClick={onPreview}
			>
				{t("ui.editor.palette.batch.preview")}
			</Button>
			<Button
				type="button"
				size="sm"
				disabled={disabled}
				aria-busy={busy}
				onClick={onRun}
			>
				{t("ui.editor.palette.batch.run")}
			</Button>
		</section>
	);
}
