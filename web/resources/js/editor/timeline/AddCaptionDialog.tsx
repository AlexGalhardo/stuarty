import { type ReactNode, useState } from "react";

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
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { useActions } from "../commands/useActions";

const DEFAULT_CAPTION_DURATION = "3s";

// The Add caption dialog the timeline toolbar and the shortcut open; it sends
// one batch (SDD-012 section 8.2). The strip of Add text and Add shape that
// stood beside it is gone: those two are in the Text tab (epic 60).
export default function AddCaptionDialog({
	open,
	onOpenChange,
}: {
	open: boolean;
	onOpenChange(open: boolean): void;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const [text, setText] = useState("");
	const [duration, setDuration] = useState(DEFAULT_CAPTION_DURATION);

	async function submit(): Promise<void> {
		onOpenChange(false);
		await actions.addCaption(text.trim(), duration.trim());
		setText("");
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t("ui.editor.caption.title")}</DialogTitle>
				</DialogHeader>
				<form
					className="flex flex-col gap-3"
					onSubmit={(event) => {
						event.preventDefault();
						void submit();
					}}
				>
					<div className="flex flex-col gap-1">
						<Label htmlFor="caption-text">{t("ui.editor.caption.text")}</Label>
						<Textarea
							id="caption-text"
							value={text}
							onChange={(event) => setText(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="caption-duration">
							{t("ui.editor.caption.duration")}
						</Label>
						<Input
							id="caption-duration"
							value={duration}
							onChange={(event) => setDuration(event.target.value)}
						/>
					</div>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							{t("ui.editor.caption.cancel")}
						</Button>
						<Button
							type="submit"
							disabled={text.trim() === "" || duration.trim() === ""}
						>
							{t("ui.editor.caption.add")}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
}
