import { type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	DEFAULT_ATTACK,
	DEFAULT_GAIN_DB,
	DEFAULT_RELEASE,
	duckOperations,
} from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import NumberField from "../fields/NumberField";
import TimeCommitField from "../fields/TimeCommitField";
import Section from "../Section";

// The Ducking section of an audio item (SDD-012 section 12.1, BETA): the
// person picks the speech items and the choices, and one button sends
// `audio.duck`. The service analyses the speech and resolves the ranges; the
// volume keyframes the core writes appear in the Keyframes section below.
export default function DuckingSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const exposed = useCommandsExposed("audio.duck");
	const items = useDocumentState((s) => s.document?.items);
	const assets = useDocumentState((s) => s.document?.assets);
	const [speech, setSpeech] = useState<readonly string[]>([]);
	const [gain, setGain] = useState(DEFAULT_GAIN_DB);
	const [attack, setAttack] = useState(DEFAULT_ATTACK);
	const [release, setRelease] = useState(DEFAULT_RELEASE);
	const [busy, setBusy] = useState(false);
	if (!exposed || items === undefined) return null;
	const candidates = Object.entries(items).filter(
		([id, other]) =>
			id !== itemId && (other.type === "clip" || other.type === "audio"),
	);
	return (
		<Section title={t("ui.editor.audio.duck_title")}>
			<fieldset className="flex flex-col gap-1">
				<legend className="text-xs">{t("ui.editor.audio.duck_speech")}</legend>
				{candidates.length === 0 && (
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.audio.duck_none")}
					</p>
				)}
				{candidates.map(([id, other]) => (
					<label key={id} className="flex items-center gap-2 text-sm">
						<input
							type="checkbox"
							checked={speech.includes(id)}
							disabled={!canEdit}
							onChange={(event) =>
								setSpeech((was) =>
									event.target.checked
										? [...was, id]
										: was.filter((entry) => entry !== id),
								)
							}
						/>
						<UntrustedText>
							{other.name ??
								assets?.[other.asset ?? ""]?.name ??
								t(`ui.editor.item.${other.type}`)}
						</UntrustedText>
					</label>
				))}
			</fieldset>
			<NumberField
				label={t("ui.editor.audio.duck_gain")}
				stored={String(gain)}
				disabled={!canEdit}
				step={1}
				min={-40}
				max={-1}
				suffix="dB"
				commit={async (value) => {
					setGain(value);
					return { ok: true };
				}}
			/>
			<TimeCommitField
				label={t("ui.editor.audio.duck_attack")}
				stored={attack}
				disabled={!canEdit}
				commit={async (value) => {
					setAttack(value);
					return { ok: true };
				}}
			/>
			<TimeCommitField
				label={t("ui.editor.audio.duck_release")}
				stored={release}
				disabled={!canEdit}
				commit={async (value) => {
					setRelease(value);
					return { ok: true };
				}}
			/>
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={!canEdit || busy || speech.length === 0}
				onClick={async () => {
					setBusy(true);
					try {
						await actions.runOperations(
							duckOperations(itemId, {
								speechItemIds: speech,
								gainDb: gain,
								attack,
								release,
							}),
						);
					} finally {
						setBusy(false);
					}
				}}
			>
				{t("ui.editor.audio.duck_apply")}
			</Button>
		</Section>
	);
}
