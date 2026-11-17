import { type ReactNode, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	captionImportSubtitlesOperations,
	subtitleFormatOf,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import type { FieldError } from "../fields/commitField";
import { FieldMessage } from "../fields/FieldFrame";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// Import subtitles (SDD-012 section 12.1): a picker for an SRT or WebVTT file.
// The browser only reads the text; `caption.import_subtitles` parses it and
// creates one caption per cue on the track. A refusal shows as received.
export default function ImportSubtitles({
	trackId,
}: {
	trackId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const input = useRef<HTMLInputElement | null>(null);
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState<FieldError | null>(null);

	async function pick(file: File): Promise<void> {
		setBusy(true);
		try {
			let text: string;
			try {
				text = await file.text();
			} catch {
				setError({
					code: "E_VALIDATION",
					message: t("ui.editor.subtitles.read_failed", { name: file.name }),
				});
				return;
			}
			const result = await send(
				captionImportSubtitlesOperations({
					trackId,
					text,
					format: subtitleFormatOf(file.name),
				}),
			);
			setError(result.ok ? null : result.error);
		} finally {
			setBusy(false);
		}
	}

	return (
		<Section title={t("ui.editor.subtitles.title")} defaultOpen={false}>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.subtitles.hint")}
			</p>
			<input
				ref={input}
				type="file"
				accept=".srt,.vtt"
				hidden
				aria-label={t("ui.editor.subtitles.file")}
				data-subtitles-file=""
				onChange={(event) => {
					const file = event.target.files?.[0];
					// The same file can be picked again after a refusal.
					event.target.value = "";
					if (file !== undefined) void pick(file);
				}}
			/>
			<Button
				type="button"
				size="sm"
				variant="outline"
				className="self-start"
				disabled={!canEdit || busy}
				aria-busy={busy}
				onClick={() => input.current?.click()}
			>
				{t("ui.editor.subtitles.import")}
			</Button>
			<FieldMessage
				id={`subtitles-message-${trackId}`}
				state={{ error, remoteActor: null }}
			/>
		</Section>
	);
}
