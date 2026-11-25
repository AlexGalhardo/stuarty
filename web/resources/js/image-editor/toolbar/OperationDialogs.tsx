import type { ReactNode } from "react";

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
import { ModelEditDialog } from "../modeledit/ModelEditDialog";
import { findLayer, useImage } from "../state/context";
import { useLibrary } from "../state/library";
import { useAddImage } from "./addImage";
import { useOperationDialogs } from "./operations";

// The dialogs the operations open: the picker of a workspace image, the import of a file and the operation with
// a model. They are drawn once, outside the buttons, so the tool rail and the command palette open the same one.
export function OperationDialogs(): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const busy = useImage((s) => s.busy);
	const send = useImage((s) => s.send);
	const importFile = useImage((s) => s.importFile);
	const error = useImage((s) => s.error);
	const selection = useImage((s) => s.selection);
	const library = useLibrary();
	const { picking, setPicking } = useAddImage();
	const { importing, setImporting, modelOperation, setModelOperation } =
		useOperationDialogs();
	const created = (r: { created: string[] }): string[] => r.created.slice(0, 1);

	const only = selection.length === 1 ? selection[0] : undefined;
	const layer =
		doc === null || only === undefined
			? undefined
			: findLayer(doc.layers, only);
	// A generation may cover the selected layer when it has a box of its own (a group has none).
	const coverable = layer !== undefined && layer.kind !== "group";

	return (
		<>
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
			{modelOperation !== null && (
				<ModelEditDialog
					operation={modelOperation}
					layerId={coverable ? (layer?.id ?? null) : null}
					open
					onOpenChange={(next) => {
						if (!next) setModelOperation(null);
					}}
				/>
			)}
		</>
	);
}
