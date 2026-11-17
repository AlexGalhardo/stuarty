import { X } from "lucide-react";
import { type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { ImportedFont } from "../../api/types";
import {
	fontAddOperations,
	fontRemoveOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import Section from "../Section";
import { useBrandKit } from "../useBrandKit";
import { useFieldCommit } from "../useFieldCommit";
import { useFonts } from "../useFonts";

const WEIGHTS = [100, 200, 300, 400, 500, 600, 700, 800, 900] as const;

// A stored font file as the picker shows it: the start of its hash and its size.
export function importedLabel(file: ImportedFont): string {
	return `${file.hash.slice(0, 19)}… (${Math.max(1, Math.round(file.bytes / 1024))} KB)`;
}

// The Fonts section of the project panel (SDD-012 section 12.1, BETA): the
// user families of the project with a remove button each (`font.remove`), and
// a form that names a font file already stored in the workspace by its hash
// (`font.add`). The browser never reads or uploads a font file; the service
// refuses a hash the workspace does not hold, and `E_IN_USE` names the styles
// that still use a family.
export default function FontsSection(): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("font.add", "font.remove");
	const fonts = useDocumentState((s) => s.document?.fonts);
	const brandFonts = Object.entries(useBrandKit()?.fonts ?? {})
		.filter(([name]) => fonts?.[name] === undefined)
		.sort(([a], [b]) => a.localeCompare(b));
	const { imported } = useFonts();
	const [family, setFamily] = useState("");
	const [weight, setWeight] = useState("400");
	const [hash, setHash] = useState("");
	const [error, setError] = useState<string | null>(null);
	if (!exposed) return null;
	const families = Object.entries(fonts ?? {}).sort(([a], [b]) =>
		a.localeCompare(b),
	);
	return (
		<Section title={t("ui.editor.fonts.title")} defaultOpen={false}>
			<ul className="flex flex-col gap-1">
				{families.map(([name, font]) => (
					<li key={name} className="flex items-center gap-1">
						<span className="min-w-0 flex-1 text-sm">
							<UntrustedText>{name}</UntrustedText>{" "}
							<span className="text-xs text-muted-foreground">
								{font.files.map((file) => file.weight).join(", ")}
							</span>
						</span>
						<Button
							type="button"
							variant="ghost"
							size="icon-sm"
							disabled={!canEdit}
							aria-label={t("ui.editor.fonts.remove", { name })}
							onClick={() => void send(fontRemoveOperations(name))}
						>
							<X aria-hidden="true" />
						</Button>
					</li>
				))}
			</ul>
			{brandFonts.length > 0 && (
				<div className="flex flex-col gap-1">
					<p className="text-xs font-medium">{t("ui.editor.fonts.brand")}</p>
					<ul className="flex flex-col gap-1" data-testid="brand-fonts">
						{brandFonts.map(([name, font]) => (
							<li key={name} className="flex items-center gap-1">
								<span className="min-w-0 flex-1 text-sm">
									<UntrustedText>{name}</UntrustedText>
								</span>
								<Button
									type="button"
									variant="outline"
									size="sm"
									disabled={!canEdit}
									aria-label={t("ui.editor.fonts.brand_add", { name })}
									onClick={() => void send(fontAddOperations(name, font.files))}
								>
									{t("ui.editor.fonts.brand_use")}
								</Button>
							</li>
						))}
					</ul>
				</div>
			)}
			<div className="flex flex-col gap-1">
				<Label htmlFor="font-family" className="text-xs">
					{t("ui.editor.fonts.family")}
				</Label>
				<Input
					id="font-family"
					value={family}
					maxLength={60}
					disabled={!canEdit}
					className="h-8"
					onChange={(event) => setFamily(event.target.value)}
				/>
			</div>
			<div className="flex flex-col gap-1">
				<Label htmlFor="font-weight" className="text-xs">
					{t("ui.editor.fonts.weight")}
				</Label>
				<select
					id="font-weight"
					value={weight}
					disabled={!canEdit}
					className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
					onChange={(event) => setWeight(event.target.value)}
				>
					{WEIGHTS.map((value) => (
						<option key={value} value={String(value)}>
							{value}
						</option>
					))}
				</select>
			</div>
			{imported.length > 0 && (
				<div className="flex flex-col gap-1">
					<Label htmlFor="font-imported" className="text-xs">
						{t("ui.editor.fonts.imported")}
					</Label>
					<select
						id="font-imported"
						value={imported.some((file) => file.hash === hash) ? hash : ""}
						disabled={!canEdit}
						className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
						onChange={(event) => setHash(event.target.value)}
					>
						<option value="">{t("ui.editor.fonts.imported_choose")}</option>
						{imported.map((file) => (
							<option key={file.hash} value={file.hash}>
								{importedLabel(file)}
							</option>
						))}
					</select>
				</div>
			)}
			<div className="flex flex-col gap-1">
				<Label htmlFor="font-hash" className="text-xs">
					{t("ui.editor.fonts.hash")}
				</Label>
				<Input
					id="font-hash"
					value={hash}
					disabled={!canEdit}
					className="h-8 font-mono"
					aria-invalid={error === null ? undefined : true}
					onChange={(event) => setHash(event.target.value.trim())}
				/>
				{error !== null && (
					<p role="alert" className="text-xs text-destructive">
						<UntrustedText>{error}</UntrustedText>
					</p>
				)}
			</div>
			<Button
				type="button"
				size="sm"
				disabled={!canEdit || family === "" || hash === ""}
				onClick={() =>
					void send(
						fontAddOperations(family, [{ weight: Number(weight), hash }]),
					).then((result) => {
						if (result.ok) {
							setError(null);
							setFamily("");
							setHash("");
						} else setError(result.error.message);
					})
				}
			>
				{t("ui.editor.fonts.add")}
			</Button>
		</Section>
	);
}
