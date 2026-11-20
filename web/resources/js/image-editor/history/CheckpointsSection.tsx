import { type FormEvent, type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { ApiError } from "../../editor/api/errors";
import type { Checkpoint } from "../../editor/api/types";
import ErrorBody from "../../editor/commands/ErrorBody";
import { useImage, useImageApi } from "../state/context";
import type { RevertTarget } from "./RevertDialog";

// The named versions of the image (RF-121): a person or an agent names a point to come back to, and the service
// names one before the first edit of the day of an agent. The list is the one of the core; the revision on
// screen makes a new step read it again.
export default function CheckpointsSection({
	onRevert,
}: {
	onRevert(target: RevertTarget): void;
}): ReactNode {
	const t = useT();
	const api = useImageApi();
	const imageId = useImage((s) => s.imageId);
	const revision = useImage((s) => s.doc?.revision ?? 0);
	const [list, setList] = useState<readonly Checkpoint[]>([]);
	const [name, setName] = useState("");
	const [error, setError] = useState<ApiError | null>(null);
	const [busy, setBusy] = useState(false);
	const [version, setVersion] = useState(0);

	// biome-ignore lint/correctness/useExhaustiveDependencies: `revision` and `version` only trigger the read
	useEffect(() => {
		let live = true;
		api
			.checkpoints(imageId)
			.then((result) => {
				if (live) setList(result.checkpoints);
			})
			.catch(() => {
				// The list is a convenience: a failed read shows the empty state, and the next step reads it again.
			});
		return () => {
			live = false;
		};
	}, [api, imageId, revision, version]);

	const add = (event: FormEvent): void => {
		event.preventDefault();
		const trimmed = name.trim();
		if (trimmed === "" || busy) return;
		setBusy(true);
		setError(null);
		api
			.createCheckpoint(imageId, trimmed)
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
		api
			.removeCheckpoint(imageId, checkpoint.name)
			.then(() => setVersion((v) => v + 1))
			.catch((e: unknown) => {
				if (e instanceof ApiError) setError(e);
			});
	};

	return (
		<section
			aria-label={t("ui.image.history.checkpoints_title")}
			data-testid="image-checkpoints"
			className="flex flex-col gap-2 border-b border-border px-3 py-2"
		>
			<h3 className="text-sm font-semibold">
				{t("ui.image.history.checkpoints_title")}
			</h3>
			<form className="flex items-end gap-2" onSubmit={add}>
				<div className="flex min-w-0 flex-1 flex-col gap-1">
					<Label htmlFor="image-checkpoint-name" className="text-xs">
						{t("ui.image.history.checkpoint_label")}
					</Label>
					<Input
						id="image-checkpoint-name"
						data-testid="image-checkpoint-name"
						value={name}
						maxLength={60}
						onChange={(e) => setName(e.target.value)}
					/>
				</div>
				<Button
					type="submit"
					size="sm"
					variant="secondary"
					data-testid="image-checkpoint-add"
					disabled={busy || name.trim() === ""}
				>
					{t("ui.image.history.checkpoint_add")}
				</Button>
			</form>
			{error !== null && (
				<div role="alert" className="flex flex-col gap-1 text-sm">
					<ErrorBody error={error} />
				</div>
			)}
			{list.length === 0 ? (
				<p className="text-xs text-muted-foreground">
					{t("ui.image.history.checkpoints_empty")}
				</p>
			) : (
				<ul className="flex flex-col divide-y divide-border">
					{list.map((cp) => (
						<li
							key={cp.name}
							data-checkpoint={cp.name}
							className="flex items-center gap-2 py-1"
						>
							<span className="font-mono text-xs tabular-nums text-muted-foreground">
								{t("ui.image.history.revision", { revision: cp.revision })}
							</span>
							<span lang="en" className="min-w-0 flex-1 truncate text-sm">
								<UntrustedText>{cp.name}</UntrustedText>
							</span>
							{cp.auto && (
								<Badge variant="outline">
									{t("ui.image.history.checkpoint_auto")}
								</Badge>
							)}
							<Button
								type="button"
								size="sm"
								variant="ghost"
								data-testid="image-checkpoint-revert"
								aria-label={t("ui.image.history.checkpoint_revert", {
									name: cp.name,
								})}
								onClick={() => onRevert({ checkpoint: cp.name })}
							>
								{t("ui.image.history.revert_here")}
							</Button>
							<Button
								type="button"
								size="sm"
								variant="ghost"
								aria-label={t("ui.image.history.checkpoint_remove", {
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
