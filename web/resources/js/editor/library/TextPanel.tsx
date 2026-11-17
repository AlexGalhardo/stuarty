import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { presetOperations, textStyleOperations } from "../commands/builders";
import { fontFamilyChanges } from "../commands/fontWeight";
import { useActions, useCanEdit } from "../commands/useActions";
import {
	useCommandEnum,
	useCommandNames,
} from "../commands/useExposedCommands";
import { useFonts } from "../commands/useFonts";
import { useDocumentState } from "../state/EditorContext";
import PanelFrame from "./PanelFrame";
import { useApplyTarget } from "./useApplyTarget";

// The Text tab of the material rail (SDD-012 section 6.4): add text, add a
// shape, the animation presets and the fonts. The lists come from the core (the
// registry enums and `getFonts`); each card sends one command.
export default function TextPanel(): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const names = useCommandNames();
	const presets = useCommandEnum("text.apply_preset", "preset");
	const shapes = useCommandEnum("shape.add", "shape");
	const { families } = useFonts();
	const target = useApplyTarget(["clip", "text", "caption", "shape"]);
	const texts = useApplyTarget(["text"]);
	const fontTarget = texts.ids.length === 1 ? texts.ids[0] : undefined;
	const stored = useDocumentState((s) =>
		fontTarget === undefined
			? undefined
			: s.document?.items[fontTarget]?.style?.font_weight,
	);
	return (
		<PanelFrame title={t("ui.editor.panels.text")}>
			<div className="flex gap-2">
				{names.has("text.add") && (
					<Button
						type="button"
						variant="secondary"
						size="sm"
						disabled={!canEdit}
						onClick={() => void actions.addText(t("ui.editor.defaults.text"))}
					>
						{t("ui.editor.library.add_text")}
					</Button>
				)}
			</div>
			{names.has("shape.add") && shapes.length > 0 && (
				<>
					<h3 className="text-xs text-muted-foreground">
						{t("ui.editor.library.shapes")}
					</h3>
					<ul className="flex flex-wrap gap-2">
						{shapes.map((shape) => (
							<li key={shape}>
								<Button
									type="button"
									variant="secondary"
									size="sm"
									disabled={!canEdit}
									onClick={() =>
										void actions.addShape(
											shape as Parameters<typeof actions.addShape>[0],
										)
									}
								>
									{t(`ui.editor.shape.${shape}`)}
								</Button>
							</li>
						))}
					</ul>
				</>
			)}
			{names.has("text.apply_preset") && presets.length > 0 && (
				<>
					<h3 className="text-xs text-muted-foreground">
						{t("ui.editor.library.presets")}
					</h3>
					{!target.canApply && (
						<p id="text-preset-why" className="text-xs text-muted-foreground">
							{t("ui.editor.library.select_first")}
						</p>
					)}
					<ul className="flex flex-col gap-1">
						{presets.map((preset) => (
							<li key={preset}>
								<Button
									type="button"
									variant="ghost"
									size="sm"
									className="w-full justify-start"
									aria-disabled={!target.canApply}
									aria-describedby={
										target.canApply ? undefined : "text-preset-why"
									}
									onClick={() => {
										if (!target.canApply) return;
										void actions.runOperations(
											target.ids.flatMap((itemId) =>
												presetOperations({
													itemId,
													edge: "in",
													preset: preset as Parameters<
														typeof presetOperations
													>[0]["preset"],
												}),
											),
										);
									}}
								>
									{t(`ui.editor.animation.preset_name.${preset}`)}
								</Button>
							</li>
						))}
					</ul>
				</>
			)}
			{families.length > 0 && (
				<>
					<h3 className="text-xs text-muted-foreground">
						{t("ui.editor.library.fonts")}
					</h3>
					{fontTarget === undefined && (
						<p id="text-font-why" className="text-xs text-muted-foreground">
							{t("ui.editor.library.select_text_first")}
						</p>
					)}
					<ul className="flex flex-wrap gap-1">
						{families.map((font) => (
							<li key={font.family}>
								<Button
									type="button"
									variant="outline"
									size="xs"
									aria-disabled={fontTarget === undefined || !canEdit}
									aria-describedby={
										fontTarget === undefined ? "text-font-why" : undefined
									}
									onClick={() => {
										if (fontTarget === undefined || !canEdit) return;
										void actions.runOperations(
											textStyleOperations(
												fontTarget,
												fontFamilyChanges(font.family, font.weights, stored),
											),
										);
									}}
								>
									{font.family}
								</Button>
							</li>
						))}
					</ul>
				</>
			)}
		</PanelFrame>
	);
}
