import {
	Captions,
	Scissors,
	Shapes,
	StepBack,
	StepForward,
	Trash2,
	Type,
} from "lucide-react";
import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { SHAPES } from "../commands/builders";
import {
	useActions,
	useCanCut,
	useCanEdit,
	useHasSelection,
} from "../commands/useActions";

const DEFAULT_CAPTION_DURATION = "3s";

function AddCaptionDialog({
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

// The Prototype editing buttons of SDD-012 section 8.2: split and trim at the
// playhead, and Add caption. Each sends one batch.
export default function EditToolbar({
	captionOpen,
	onCaptionOpenChange,
}: {
	captionOpen: boolean;
	onCaptionOpenChange(open: boolean): void;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const canCut = useCanCut();
	const ready = useCanEdit();
	const hasSelection = useHasSelection();
	return (
		<div
			role="toolbar"
			aria-label={t("ui.editor.edit.label")}
			className="flex items-center gap-1"
		>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!canCut}
				onClick={() => void actions.splitAtPlayhead()}
			>
				<Scissors aria-hidden="true" />
				{t("ui.editor.edit.split")}
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!canCut}
				onClick={() => void actions.trimToPlayhead("start")}
			>
				<StepForward aria-hidden="true" />
				{t("ui.editor.edit.trim_start")}
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!canCut}
				onClick={() => void actions.trimToPlayhead("end")}
			>
				<StepBack aria-hidden="true" />
				{t("ui.editor.edit.trim_end")}
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!hasSelection}
				onClick={() => void actions.deleteSelection()}
			>
				<Trash2 aria-hidden="true" />
				{t("ui.editor.edit.delete")}
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!ready}
				onClick={() => onCaptionOpenChange(true)}
			>
				<Captions aria-hidden="true" />
				{t("ui.editor.edit.caption")}
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				disabled={!ready}
				onClick={() => void actions.addText(t("ui.editor.defaults.text"))}
			>
				<Type aria-hidden="true" />
				{t("ui.editor.edit.text")}
			</Button>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button type="button" variant="ghost" size="sm" disabled={!ready}>
						<Shapes aria-hidden="true" />
						{t("ui.editor.edit.shape")}
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start">
					{SHAPES.map((shape) => (
						<DropdownMenuItem
							key={shape}
							onSelect={() => void actions.addShape(shape)}
						>
							{t(`ui.editor.shape.${shape}`)}
						</DropdownMenuItem>
					))}
				</DropdownMenuContent>
			</DropdownMenu>
			<AddCaptionDialog open={captionOpen} onOpenChange={onCaptionOpenChange} />
		</div>
	);
}
