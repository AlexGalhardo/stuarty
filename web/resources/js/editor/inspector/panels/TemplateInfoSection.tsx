import { X } from "lucide-react";
import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { templateInfoOperations } from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import ColorField from "../fields/ColorField";
import TextField from "../fields/TextField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import SlotsPanel from "./SlotsPanel";

// The Template section of the project panel (SDD-012 section 12.1): the
// description and the named colours (`template.set_info`), then the slots
// panel. A colour name is `[a-z][a-z0-9_]{0,39}` and styles refer to it as
// "$name"; the core judges the name, and refuses to remove one a style uses.
export default function TemplateInfoSection(): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const template = useDocumentState((s) => s.document?.template);
	const [name, setName] = useState("");
	const [colour, setColour] = useState("#FFFFFF");
	const colors = Object.entries(template?.colors ?? {});

	return (
		<>
			<Section title={t("ui.editor.template.info_title")}>
				<TextField
					label={t("ui.editor.template.info_description")}
					stored={template?.description ?? ""}
					disabled={!canEdit}
					maxLength={500}
					commit={(description) =>
						send(templateInfoOperations({ description }))
					}
				/>
				<ul className="flex flex-col gap-2">
					{colors.map(([key, value]) => (
						<li key={key} className="flex items-end gap-1">
							<div className="min-w-0 flex-1">
								<ColorField
									label={`$${key}`}
									stored={value}
									disabled={!canEdit}
									commit={(hex) =>
										send(templateInfoOperations({ colors: { [key]: hex } }))
									}
								/>
							</div>
							<Button
								type="button"
								variant="ghost"
								size="icon-sm"
								disabled={!canEdit}
								aria-label={t("ui.editor.template.color_remove", { name: key })}
								onClick={() =>
									void send(templateInfoOperations({ colors: { [key]: null } }))
								}
							>
								<X aria-hidden="true" />
							</Button>
						</li>
					))}
				</ul>
				<div className="flex flex-col gap-1">
					<Label htmlFor="template-color-name" className="text-xs">
						{t("ui.editor.template.color_name")}
					</Label>
					<Input
						id="template-color-name"
						value={name}
						maxLength={40}
						disabled={!canEdit}
						className="h-8"
						onChange={(event) => setName(event.target.value)}
					/>
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor="template-color-value" className="text-xs">
						{t("ui.editor.template.color_value")}
					</Label>
					<Input
						id="template-color-value"
						value={colour}
						maxLength={9}
						disabled={!canEdit}
						className="h-8 font-mono"
						onChange={(event) => setColour(event.target.value)}
					/>
				</div>
				<Button
					type="button"
					size="sm"
					disabled={!canEdit || name === ""}
					onClick={() =>
						void send(
							templateInfoOperations({ colors: { [name]: colour } }),
						).then((result) => {
							if (result.ok) setName("");
						})
					}
				>
					{t("ui.editor.template.color_add")}
				</Button>
			</Section>
			<SlotsPanel />
		</>
	);
}
