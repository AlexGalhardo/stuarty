import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";

// What a person gives to bring a file into an image (SDD-021 section 10): a file chosen in the browser, or the path
// of a file on the machine of the server, and for a new image its name and its size. The screen sends it and shows
// the answer; every rule about the file is the server's.
export interface ImportValues {
	file?: File;
	path?: string;
	name?: string;
	preset?: string;
}

const PRESETS = ["video_thumbnail", "square_post", "vertical_cover"] as const;
const FROM_FILE = "file";
const NAME_MAX = 120;

// The dialog of "Import image" (a new document) and of "Put a file in this image" (one more layer).
export default function ImportDialog({
	open,
	onOpenChange,
	mode,
	busy,
	failure,
	onImport,
}: {
	open: boolean;
	onOpenChange(open: boolean): void;
	mode: "new" | "layer";
	busy: boolean;
	failure: Failure | null;
	onImport(values: ImportValues): void;
}): ReactNode {
	const t = useT();
	const [file, setFile] = useState<File | null>(null);
	const [path, setPath] = useState("");
	const [name, setName] = useState("");
	const [size, setSize] = useState<string>(FROM_FILE);
	const [missing, setMissing] = useState(false);

	function submit(event: FormEvent<HTMLFormElement>): void {
		event.preventDefault();
		if (file === null && path.trim() === "") {
			setMissing(true);
			return;
		}
		setMissing(false);
		onImport({
			...(file === null ? { path: path.trim() } : { file }),
			...(name.trim() === "" ? {} : { name: name.trim() }),
			...(mode === "new" && size !== FROM_FILE ? { preset: size } : {}),
		});
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent data-testid="image-import-dialog">
				<DialogTitle>{t(`ui.image.import.title_${mode}`)}</DialogTitle>
				<DialogDescription>
					{t(`ui.image.import.description_${mode}`)}
				</DialogDescription>
				<form onSubmit={submit} className="flex flex-col gap-3">
					<div className="flex flex-col gap-1">
						<Label htmlFor="image-import-file">
							{t("ui.image.import.file")}
						</Label>
						<Input
							id="image-import-file"
							type="file"
							accept="image/*"
							data-testid="image-import-file"
							onChange={(event) => setFile(event.target.files?.[0] ?? null)}
						/>
						<p className="text-xs text-muted-foreground">
							{t("ui.image.import.file_help")}
						</p>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="image-import-path">
							{t("ui.image.import.path")}
						</Label>
						<Input
							id="image-import-path"
							value={path}
							onChange={(event) => setPath(event.target.value)}
							placeholder={t("ui.image.import.path_placeholder")}
							data-testid="image-import-path"
						/>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="image-import-name">
							{t("ui.image.import.name")}
						</Label>
						<Input
							id="image-import-name"
							value={name}
							maxLength={NAME_MAX}
							onChange={(event) => setName(event.target.value)}
							placeholder={t("ui.image.import.name_placeholder")}
						/>
					</div>
					{mode === "new" && (
						<div className="flex flex-col gap-1">
							<Label htmlFor="image-import-size">
								{t("ui.image.import.size")}
							</Label>
							<select
								id="image-import-size"
								value={size}
								onChange={(event) => setSize(event.target.value)}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								<option value={FROM_FILE}>
									{t("ui.image.import.size_file")}
								</option>
								{PRESETS.map((p) => (
									<option key={p} value={p}>
										{t(`ui.image.preset.${p}`)}
									</option>
								))}
							</select>
						</div>
					)}
					{missing && (
						<p role="alert" className="text-sm text-destructive">
							{t("ui.image.import.need_source")}
						</p>
					)}
					<FailureNote failure={failure} />
					<div className="flex justify-end gap-2">
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
						>
							{t("ui.image.import.cancel")}
						</Button>
						<Button
							type="submit"
							disabled={busy}
							data-testid="image-import-submit"
						>
							{busy
								? t("ui.image.import.working")
								: t("ui.image.import.submit")}
						</Button>
					</div>
				</form>
			</DialogContent>
		</Dialog>
	);
}
