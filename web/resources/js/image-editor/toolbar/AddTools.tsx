import {
	Circle,
	FileUp,
	ImagePlus,
	PaintBucket,
	Square,
	Stamp,
	Type,
} from "lucide-react";
import { type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogTitle,
} from "@/components/ui/dialog";
import { useT } from "@/lib/i18n";
import ImportDialog from "../import/ImportDialog";
import { useImage } from "../state/context";
import { useLibrary } from "../state/library";
import ToolButton from "./ToolButton";
import { ToolGroup } from "./Toolbar";

// The buttons that add a layer (SDD-012 section 24.6): text, a rectangle, an ellipse, a fill, an image of the
// library and the logo of the brand kit. Each is one `layer.add_*` command, and the new layer is selected.
export function AddTools(): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const busy = useImage((s) => s.busy);
	const send = useImage((s) => s.send);
	const importFile = useImage((s) => s.importFile);
	const error = useImage((s) => s.error);
	const library = useLibrary();
	const [picking, setPicking] = useState(false);
	const [importing, setImporting] = useState(false);
	const off = doc === null || busy;
	const logo = library.brand?.logo;
	const created = (r: { created: string[] }): string[] => r.created.slice(0, 1);
	// A shape starts a quarter of the canvas wide, centred by the core.
	const side =
		doc === null
			? 200
			: Math.max(Math.round(Math.min(doc.width, doc.height) / 4), 1);

	return (
		<ToolGroup>
			<ToolButton
				icon={Type}
				label={t("ui.image.add.text")}
				disabled={off}
				onClick={() =>
					void send(
						[
							{
								op: "layer.add_text",
								args: { text: t("ui.image.add.text_default") },
							},
						],
						created,
					)
				}
				testId="image-add-text"
			/>
			<ToolButton
				icon={Square}
				label={t("ui.image.add.rectangle")}
				disabled={off}
				onClick={() =>
					void send(
						[
							{
								op: "layer.add_shape",
								args: { shape: "rect", w: side, h: side },
							},
						],
						created,
					)
				}
				testId="image-add-rect"
			/>
			<ToolButton
				icon={Circle}
				label={t("ui.image.add.ellipse")}
				disabled={off}
				onClick={() =>
					void send(
						[
							{
								op: "layer.add_shape",
								args: { shape: "ellipse", w: side, h: side },
							},
						],
						created,
					)
				}
				testId="image-add-ellipse"
			/>
			<ToolButton
				icon={PaintBucket}
				label={t("ui.image.add.fill")}
				disabled={off}
				onClick={() =>
					void send(
						[{ op: "layer.add_fill", args: { color: "#808080" } }],
						created,
					)
				}
				testId="image-add-fill"
			/>
			<ToolButton
				icon={ImagePlus}
				label={t("ui.image.add.image")}
				disabled={off}
				onClick={() => setPicking(true)}
				testId="image-add-image"
			/>
			<ToolButton
				icon={FileUp}
				label={t("ui.image.import.open_layer")}
				disabled={off}
				onClick={() => setImporting(true)}
				testId="image-import-open"
			/>
			<ImportDialog
				open={importing}
				onOpenChange={setImporting}
				mode="layer"
				busy={busy}
				failure={
					error === null
						? null
						: { code: error.code, message: error.message, hint: error.hint }
				}
				onImport={(values) => {
					const source =
						values.file === undefined
							? { path: values.path ?? "", name: values.name }
							: { file: values.file, name: values.name };
					void importFile(source).then((result) => {
						if (result !== null) setImporting(false);
					});
				}}
			/>
			<ToolButton
				icon={Stamp}
				label={t("ui.image.brand.place_logo")}
				disabled={off || logo === undefined || logo === ""}
				onClick={() =>
					void send(
						[
							{
								op: "layer.add_image",
								args: { asset: logo, name: t("ui.image.brand.logo_name") },
							},
						],
						created,
					)
				}
				testId="image-place-logo"
			/>
			<Dialog open={picking} onOpenChange={setPicking}>
				<DialogContent data-testid="image-picker">
					<DialogTitle>{t("ui.image.add.image")}</DialogTitle>
					<DialogDescription>{t("ui.image.add.image_hint")}</DialogDescription>
					{library.images.length === 0 ? (
						<p className="text-sm text-muted-foreground">
							{t("ui.image.add.image_none")}
						</p>
					) : (
						<ul className="flex max-h-72 flex-col gap-1 overflow-y-auto">
							{library.images.map((asset) => (
								<li key={asset.hash}>
									<Button
										type="button"
										variant="outline"
										className="w-full justify-start"
										data-testid="image-picker-item"
										onClick={() => {
											setPicking(false);
											void send(
												[
													{
														op: "layer.add_image",
														args: { asset: asset.hash, name: asset.name },
													},
												],
												created,
											);
										}}
									>
										<UntrustedText>{asset.name}</UntrustedText>
										{asset.width && asset.height ? (
											<span className="ml-2 text-xs text-foreground">
												{t("ui.image.editor.size", {
													w: asset.width,
													h: asset.height,
												})}
											</span>
										) : null}
									</Button>
								</li>
							))}
						</ul>
					)}
				</DialogContent>
			</Dialog>
		</ToolGroup>
	);
}
