import { type FormEvent, type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { ApiError } from "../api/errors";
import type { Checkpoint } from "../api/types";
import ErrorBody from "../commands/ErrorBody";
import { useCanEdit } from "../commands/useActions";
import { useRuntime } from "../state/EditorContext";
import type { RevertTarget } from "./RevertDialog";

// The named states of the project (RF-121): a person or an agent names a point to
// come back to; the service names one before an agent's first edit of the day.
// The list is the core's; `head` is the newest revision, so that a step made by
// anyone refreshes it.
export default function CheckpointsSection({
	head,
	onRevert,
}: {
	head: number;
	onRevert(target: RevertTarget): void;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const canEdit = useCanEdit();
	const [list, setList] = useState<readonly Checkpoint[]>([]);
	const [name, setName] = useState("");
	const [error, setError] = useState<ApiError | null>(null);
	const [busy, setBusy] = useState(false);
	const [version, setVersion] = useState(0);

	// biome-ignore lint/correctness/useExhaustiveDependencies: `head` and `version` only trigger the read
	useEffect(() => {
		let live = true;
		runtime.client
			.getCheckpoints(runtime.projectId)
			.then((result) => {
				if (live) setList(result.checkpoints);
			})
			.catch(() => {
				// The list is a convenience: a failed read shows the empty state, and the
				// next step of the history reads it again.
			});
		return () => {
			live = false;
		};
	}, [runtime, head, version]);

	const add = (event: FormEvent): void => {
		event.preventDefault();
		const trimmed = name.trim();
		if (trimmed === "" || busy) return;
		setBusy(true);
		setError(null);
		runtime.client
			.createCheckpoint(runtime.projectId, { name: trimmed })
			.then(() => {
				setBusy(false);
				setName("");
				setVersion((v) => v + 1);
			})
			.catch((e: unknown) => {
				setBusy(false);
				if (e instanceof ApiError) setError(e);
			});
	};

	const remove = (checkpoint: Checkpoint): void => {
		setError(null);
		runtime.client
			.removeCheckpoint(runtime.projectId, checkpoint.name)
			.then(() => setVersion((v) => v + 1))
			.catch((e: unknown) => {
				if (e instanceof ApiError) setError(e);
			});
	};

	return (
		<section
			aria-label={t("ui.editor.history.checkpoints_title")}
			className="flex flex-col gap-2 border-b border-border pb-2"
		>
			<h3 className="text-sm font-semibold">
				{t("ui.editor.history.checkpoints_title")}
			</h3>
			<form className="flex items-end gap-2" onSubmit={add}>
				<div className="flex min-w-0 flex-1 flex-col gap-1">
					<Label htmlFor="checkpoint-name" className="text-xs">
						{t("ui.editor.history.checkpoint_label")}
					</Label>
					<Input
						id="checkpoint-name"
						value={name}
						maxLength={60}
						disabled={!canEdit}
						onChange={(e) => setName(e.target.value)}
					/>
				</div>
				<Button
					type="submit"
					size="sm"
					variant="secondary"
					disabled={!canEdit || busy || name.trim() === ""}
				>
					{t("ui.editor.history.checkpoint_add")}
				</Button>
			</form>
			{error !== null && (
				<div role="alert" className="flex flex-col gap-1 text-sm">
					<ErrorBody error={error} />
				</div>
			)}
			{list.length === 0 ? (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.history.checkpoints_empty")}
				</p>
			) : (
				<ul className="flex flex-col divide-y divide-border">
					{list.map((cp) => (
						<li
							key={cp.name}
							data-checkpoint={cp.name}
							className="flex items-center gap-2 py-1"
						>
							<span className="font-mono text-xs text-muted-foreground tabular-nums">
								{t("ui.editor.history.revision", { revision: cp.revision })}
							</span>
							<span lang="en" className="min-w-0 flex-1 truncate text-sm">
								<UntrustedText>{cp.name}</UntrustedText>
							</span>
							{cp.auto && (
								<Badge variant="outline">
									{t("ui.editor.history.checkpoint_auto")}
								</Badge>
							)}
							<Button
								type="button"
								size="sm"
								variant="ghost"
								disabled={!canEdit}
								aria-label={t("ui.editor.history.checkpoint_revert", {
									name: cp.name,
								})}
								onClick={() => onRevert({ checkpoint: cp.name })}
							>
								{t("ui.editor.history.revert_here")}
							</Button>
							<Button
								type="button"
								size="sm"
								variant="ghost"
								disabled={!canEdit}
								aria-label={t("ui.editor.history.checkpoint_remove", {
									name: cp.name,
								})}
								onClick={() => remove(cp)}
							>
								×
							</Button>
						</li>
					))}
				</ul>
			)}
		</section>
	);
}
