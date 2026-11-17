import { X } from "lucide-react";
import { type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { ApiError } from "../../api/errors";
import { templateInfoOperations } from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import { errorHeadline } from "../../ui/errorHeadline";
import ColorField from "../fields/ColorField";
import TextField from "../fields/TextField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import SlotsPanel from "./SlotsPanel";

// "Save as template": the template of this project is made by the service from its current state, and
// its refusals (a slot on an item that uses media, for example) are shown as received (VV-132).
function SaveAsTemplate(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const canEdit = useCanEdit();
	const [name, setName] = useState("");
	const [description, setDescription] = useState("");
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState<ApiError | null>(null);
	const [saved, setSaved] = useState<string | null>(null);

	async function save(): Promise<void> {
		setBusy(true);
		setError(null);
		setSaved(null);
		try {
			const result = await runtime.client.saveTemplate({
				project_id: runtime.projectId,
				name: name.trim(),
				...(description.trim() === ""
					? {}
					: { description: description.trim() }),
			});
			setSaved(result.template.name);
			setName("");
			setDescription("");
		} catch (failure) {
			if (failure instanceof ApiError) setError(failure);
			else throw failure;
		} finally {
			setBusy(false);
		}
	}

	return (
		<Section title={t("ui.editor.template.save_title")}>
			<div className="flex flex-col gap-1">
				<Label htmlFor="template-save-name" className="text-xs">
					{t("ui.editor.template.save_name")}
				</Label>
				<Input
					id="template-save-name"
					value={name}
					maxLength={120}
					disabled={!canEdit}
					className="h-8"
					onChange={(event) => setName(event.target.value)}
				/>
			</div>
			<div className="flex flex-col gap-1">
				<Label htmlFor="template-save-description" className="text-xs">
					{t("ui.editor.template.save_description")}
				</Label>
				<Input
					id="template-save-description"
					value={description}
					maxLength={500}
					disabled={!canEdit}
					className="h-8"
					onChange={(event) => setDescription(event.target.value)}
				/>
			</div>
			<Button
				type="button"
				size="sm"
				disabled={!canEdit || busy || name.trim() === ""}
				onClick={() => void save()}
			>
				{t("ui.editor.template.save")}
			</Button>
			{error !== null && (
				<div role="alert" className="text-xs text-destructive">
					<p className="font-semibold">{errorHeadline(t, error.code)}</p>
					<p lang="en">
						<UntrustedText>{error.message}</UntrustedText>
					</p>
				</div>
			)}
			{saved !== null && (
				<p role="status" className="text-xs">
					{t("ui.editor.template.saved")} <UntrustedText>{saved}</UntrustedText>
				</p>
			)}
		</Section>
	);
}

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
			<SaveAsTemplate />
		</>
	);
}
