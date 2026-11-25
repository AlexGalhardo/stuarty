import { type ReactNode, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	BACKGROUND_KINDS,
	type BackgroundKind,
	backgroundOperations,
	CAMERA_SHAPES,
	type CameraShape,
	CORNERS,
	type Corner,
	LAYOUTS,
	type Layout,
	layoutOperations,
} from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import Section from "../Section";

const isLayout = (value: string): value is Layout =>
	(LAYOUTS as readonly string[]).includes(value);
const isCorner = (value: string): value is Corner =>
	(CORNERS as readonly string[]).includes(value);
const isShape = (value: string): value is CameraShape =>
	(CAMERA_SHAPES as readonly string[]).includes(value);
const isKind = (value: string): value is BackgroundKind =>
	(BACKGROUND_KINDS as readonly string[]).includes(value);

// The Layout section of a clip (SDD-025 section 20, SDD-002 C.96 and C.97): the ready layouts for a screen clip and a
// webcam clip, and the background behind the screen. The clip of the inspector is the screen; the webcam is any other
// clip of the project the person picks. Each button sends one command; the core places the clips for the format of the
// project and answers with a refusal when it cannot, and every value it writes stays editable.
export default function RecordingLayoutSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const layoutExposed = useCommandsExposed("recording.apply_layout");
	const backgroundExposed = useCommandsExposed("recording.set_background");
	const document = useDocumentState((s) => s.document);
	const [layout, setLayout] = useState<Layout>("pip");
	const [camera, setCamera] = useState("");
	const [corner, setCorner] = useState<Corner>("bottom_right");
	const [shape, setShape] = useState<CameraShape>("circle");
	const [padding, setPadding] = useState<number | undefined>(undefined);
	const [kind, setKind] = useState<BackgroundKind>("color");
	const [color, setColor] = useState("#0F172A");
	const [image, setImage] = useState("");
	const [busy, setBusy] = useState(false);

	const clips = useMemo(
		() =>
			Object.values(document?.items ?? {})
				.filter((it) => it.type === "clip" && it.id !== itemId)
				.map((it) => ({
					value: it.id,
					label:
						it.name ??
						(it.asset === undefined
							? it.id
							: document?.assets[it.asset]?.name) ??
						it.id,
				})),
		[document, itemId],
	);
	const images = useMemo(
		() =>
			Object.values(document?.assets ?? {})
				.filter((a) => a.kind === "image")
				.map((a) => ({ value: a.id, label: a.name })),
		[document],
	);

	if (!layoutExposed && !backgroundExposed) return null;

	const withCamera = layout !== "screen_only";
	const cameraId = camera !== "" ? camera : clips[0]?.value;
	const run = async (
		operations: Parameters<typeof actions.runOperations>[0],
	): Promise<void> => {
		setBusy(true);
		try {
			await actions.runOperations(operations);
		} finally {
			setBusy(false);
		}
	};

	return (
		<Section title={t("ui.editor.layout.title")} name="recording-layout">
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.layout.hint")}
			</p>
			{layoutExposed && (
				<>
					<SelectField
						label={t("ui.editor.layout.layout")}
						stored={layout}
						disabled={!canEdit}
						options={[
							{
								value: "screen_only",
								label: t("ui.editor.layout.screen_only"),
							},
							{ value: "pip", label: t("ui.editor.layout.pip") },
							{ value: "split", label: t("ui.editor.layout.split") },
							{
								value: "camera_focus",
								label: t("ui.editor.layout.camera_focus"),
							},
						]}
						commit={async (value) => {
							if (isLayout(value)) setLayout(value);
							return { ok: true };
						}}
					/>
					{withCamera && (
						<SelectField
							label={t("ui.editor.layout.camera")}
							stored={cameraId ?? ""}
							disabled={!canEdit || clips.length === 0}
							options={
								clips.length === 0
									? [{ value: "", label: t("ui.editor.layout.no_camera") }]
									: clips
							}
							commit={async (value) => {
								setCamera(value);
								return { ok: true };
							}}
						/>
					)}
					{(layout === "pip" || layout === "camera_focus") && (
						<SelectField
							label={t("ui.editor.layout.corner")}
							stored={corner}
							disabled={!canEdit}
							options={[
								{
									value: "top_left",
									label: t("ui.editor.layout.top_left"),
								},
								{
									value: "top_right",
									label: t("ui.editor.layout.top_right"),
								},
								{
									value: "bottom_left",
									label: t("ui.editor.layout.bottom_left"),
								},
								{
									value: "bottom_right",
									label: t("ui.editor.layout.bottom_right"),
								},
							]}
							commit={async (value) => {
								if (isCorner(value)) setCorner(value);
								return { ok: true };
							}}
						/>
					)}
					{layout === "pip" && (
						<SelectField
							label={t("ui.editor.layout.shape")}
							stored={shape}
							disabled={!canEdit}
							options={[
								{ value: "circle", label: t("ui.editor.layout.circle") },
								{ value: "rect", label: t("ui.editor.layout.rect") },
							]}
							commit={async (value) => {
								if (isShape(value)) setShape(value);
								return { ok: true };
							}}
						/>
					)}
					<NumberField
						label={t("ui.editor.layout.padding")}
						stored={padding === undefined ? "" : String(padding)}
						disabled={!canEdit}
						step={1}
						min={0}
						max={25}
						suffix="%"
						placeholder={t("ui.editor.layout.default")}
						commit={async (value) => {
							setPadding(value);
							return { ok: true };
						}}
					/>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={
							!canEdit || busy || (withCamera && cameraId === undefined)
						}
						onClick={() =>
							run(
								layoutOperations(itemId, {
									layout,
									cameraItemId: withCamera ? cameraId : undefined,
									corner,
									cameraShape: shape,
									paddingPercent: padding,
								}),
							)
						}
					>
						{t("ui.editor.layout.apply")}
					</Button>
				</>
			)}
			{backgroundExposed && (
				<>
					<SelectField
						label={t("ui.editor.layout.background")}
						stored={kind}
						disabled={!canEdit}
						options={[
							{ value: "color", label: t("ui.editor.layout.bg_color") },
							{ value: "image", label: t("ui.editor.layout.bg_image") },
							{ value: "none", label: t("ui.editor.layout.bg_none") },
						]}
						commit={async (value) => {
							if (isKind(value)) setKind(value);
							return { ok: true };
						}}
					/>
					{kind === "color" && (
						<label className="flex items-center gap-2 text-sm">
							{t("ui.editor.layout.bg_color_value")}
							<input
								type="color"
								value={color}
								disabled={!canEdit}
								onChange={(event) => setColor(event.target.value)}
							/>
						</label>
					)}
					{kind === "image" && (
						<SelectField
							label={t("ui.editor.layout.bg_image_asset")}
							stored={image}
							disabled={!canEdit || images.length === 0}
							options={
								images.length === 0
									? [{ value: "", label: t("ui.editor.layout.no_image") }]
									: [
											{ value: "", label: t("ui.editor.layout.pick_image") },
											...images,
										]
							}
							commit={async (value) => {
								setImage(value);
								return { ok: true };
							}}
						/>
					)}
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={!canEdit || busy || (kind === "image" && image === "")}
						onClick={() =>
							run(
								backgroundOperations(itemId, {
									kind,
									color,
									assetId: image === "" ? undefined : image,
								}),
							)
						}
					>
						{t("ui.editor.layout.bg_apply")}
					</Button>
				</>
			)}
		</Section>
	);
}
