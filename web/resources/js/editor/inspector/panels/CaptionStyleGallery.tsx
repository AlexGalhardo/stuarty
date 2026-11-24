import { type ReactNode, useId, useState } from "react";

import { useT } from "@/lib/i18n";
import { captionApplyStyleOperations } from "../../commands/builders";
import {
	CAPTION_STYLES,
	captionStyleSample,
} from "../../commands/captionStyles";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";

// The style gallery of a caption track (SDD-012 section 12.1, SDD-002 C.88): one tile for each named style of the
// core, with a sample, its name and its font. A tile sends `caption.apply_style` with the style id; the core
// resolves the style and the preview shows the result. The styles come from the registry, not from this file.
export default function CaptionStyleGallery({
	trackId,
}: {
	trackId: string;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const exposed = useCommandsExposed("caption.apply_style");
	const resetId = useId();
	const [reset, setReset] = useState(false);
	const [busy, setBusy] = useState(false);
	if (!exposed || CAPTION_STYLES.length === 0) return null;
	return (
		<div className="flex flex-col gap-2" data-testid="caption-style-gallery">
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.caption_gallery.hint")}
			</p>
			<ul className="grid grid-cols-2 gap-2">
				{CAPTION_STYLES.map((style) => {
					const name = t(`ui.editor.caption_gallery.styles.${style.id}`);
					return (
						<li key={style.id}>
							<button
								type="button"
								disabled={!canEdit || busy}
								aria-label={t("ui.editor.caption_gallery.apply", { name })}
								data-style-id={style.id}
								className="flex w-full flex-col gap-1 rounded-md border border-border p-1 text-left hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-50"
								onClick={async () => {
									setBusy(true);
									try {
										await actions.runOperations(
											captionApplyStyleOperations(trackId, style.id, reset),
										);
									} finally {
										setBusy(false);
									}
								}}
							>
								{/* The specimen sits on the dark canvas in both themes, as a caption sits on footage:
								    a white caption on the light canvas would hide the look it shows. It is a
								    picture of the style (WCAG 1.4.3, incidental text); the name below is the text. */}
								<span className="dark flex h-14 items-center justify-center overflow-hidden rounded-sm bg-ed-preview-surround px-1">
									<span
										aria-hidden="true"
										data-caption-sample=""
										style={captionStyleSample(style.caption_style)}
									>
										{t("ui.editor.caption_gallery.sample")}
									</span>
								</span>
								<span className="text-xs font-medium">{name}</span>
								<span className="text-xs text-muted-foreground">
									{style.font_family}
								</span>
							</button>
						</li>
					);
				})}
			</ul>
			<div className="flex items-center gap-2 text-xs">
				<input
					id={resetId}
					type="checkbox"
					checked={reset}
					disabled={!canEdit}
					className="size-4 accent-primary"
					onChange={(event) => setReset(event.target.checked)}
				/>
				<label htmlFor={resetId}>
					{t("ui.editor.caption_gallery.reset_overrides")}
				</label>
			</div>
		</div>
	);
}
