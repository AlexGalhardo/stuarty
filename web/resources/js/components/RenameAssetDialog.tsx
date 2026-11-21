import { type FormEvent, type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";

// The name of an imported file, changed where the person sees it (epic 60, SDD-011
// section 14.8). The dialog sends what was typed and shows the refusal of the
// service as it came; the rules of the name (length, separators, the extension
// that is kept) are the service's, never checked here.
export default function RenameAssetDialog({
	open,
	onOpenChange,
	name,
	rename,
}: {
	open: boolean;
	onOpenChange(open: boolean): void;
	// The name the file has now, offered for editing.
	name: string;
	// Sends the new name; a refusal of the service is an ApiError.
	rename(name: string): Promise<void>;
}): ReactNode {
	const t = useT();
	const [value, setValue] = useState(name);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);

	// Each opening starts from the name the file has then, with no old refusal.
	useEffect(() => {
		if (open) {
			setValue(name);
			setFailure(null);
		}
	}, [open, name]);

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			await rename(value);
			onOpenChange(false);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t("ui.editor.library.rename.title")}</DialogTitle>
				</DialogHeader>
				<form className="flex flex-col gap-3" onSubmit={submit}>
					<div className="flex flex-col gap-1">
						<Label htmlFor="rename-asset-name">
							{t("ui.editor.library.rename.label")}
						</Label>
						<Input
							id="rename-asset-name"
							value={value}
							autoFocus
							aria-describedby="rename-asset-hint"
							onFocus={(event) => event.currentTarget.select()}
							onChange={(event) => setValue(event.target.value)}
						/>
						<p id="rename-asset-hint" className="text-xs text-muted-foreground">
							{t("ui.editor.library.rename.hint")}
						</p>
						<FailureNote failure={failure} />
					</div>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							{t("ui.editor.library.rename.cancel")}
						</Button>
						<Button type="submit" disabled={busy || value.trim() === ""}>
							{t("ui.editor.library.rename.submit")}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
}
