import { type FormEvent, type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { ImageApi } from "../api/imageApi";
import { type Library, loadLibrary } from "../api/library";
import type { Finding, ImageSummary, Layer } from "../api/types";

interface Slot {
	name: string;
	kind: "text" | "image";
	// A slot with no content in the template is required (SDD-021 section 7).
	required: boolean;
	layerName: string;
}

// The lint finding of the core for a slot with no content: the core decides which slots are required.
const EMPTY_SLOT = "L_IMAGE_EMPTY_SLOT";

// The slots of a template, in the order of its layers, read from what the server described. A slot is
// required when the core's lint says it is empty, never by a rule of its own here.
export function slotsOf(
	layers: readonly Layer[],
	findings: readonly Finding[],
): Slot[] {
	const empty = new Set(
		findings
			.filter((f) => f.code === EMPTY_SLOT && f.item !== undefined)
			.map((f) => f.item),
	);
	const out: Slot[] = [];
	const visit = (list: readonly Layer[]): void => {
		for (const layer of list) {
			if (
				layer.slot !== undefined &&
				layer.slot !== "" &&
				(layer.kind === "text" || layer.kind === "image")
			) {
				out.push({
					name: layer.slot,
					kind: layer.kind,
					required: empty.has(layer.id),
					layerName: layer.name,
				});
			}
			if (layer.children) visit(layer.children);
		}
	};
	visit(layers);
	return out;
}

type Values = { [slot: string]: { text?: string; asset?: string } };

// "New from template" (RF-342, SDD-012 section 24.7): the image templates, the value of each slot, and the
// document the server makes from them.
export default function NewFromTemplate({ api }: { api: ImageApi }): ReactNode {
	const t = useT();
	const [templates, setTemplates] = useState<ImageSummary[]>([]);
	const [library, setLibrary] = useState<Library | null>(null);
	const [chosen, setChosen] = useState("");
	const [slots, setSlots] = useState<Slot[]>([]);
	const [values, setValues] = useState<Values>({});
	const [name, setName] = useState("");
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);

	useEffect(() => {
		let live = true;
		api
			.list("only")
			.then((r) => live && setTemplates(r.images))
			.catch(() => undefined);
		loadLibrary()
			.then((l) => live && setLibrary(l))
			.catch(() => undefined);
		return () => {
			live = false;
		};
	}, [api]);

	async function pick(id: string): Promise<void> {
		setChosen(id);
		setValues({});
		setFailure(null);
		if (id === "") {
			setSlots([]);
			return;
		}
		try {
			const template = await api.describe(id);
			setSlots(slotsOf(template.layers, template.findings));
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	async function create(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		const filled: Values = {};
		for (const [slot, value] of Object.entries(values)) {
			if ((value.text ?? "") !== "" || (value.asset ?? "") !== "") {
				filled[slot] = value;
			}
		}
		try {
			const made = await api.create({
				template_id: chosen,
				values: filled,
				...(name.trim() === "" ? {} : { name: name.trim() }),
			});
			window.location.assign(`/edit-image/${made.image_id}`);
		} catch (error) {
			setFailure(failureOf(error));
			setBusy(false);
		}
	}

	return (
		<section
			aria-labelledby="image-template-title"
			className="flex flex-col gap-3"
		>
			<h2 id="image-template-title" className="text-lg font-medium">
				{t("ui.image.template.title")}
			</h2>
			{templates.length === 0 ? (
				<p className="text-sm text-muted-foreground">
					{t("ui.image.template.none")}
				</p>
			) : (
				<form
					onSubmit={(event) => void create(event)}
					className="flex flex-col gap-3"
					data-testid="image-template-form"
				>
					<div className="flex flex-wrap items-end gap-3">
						<div className="flex flex-col gap-1">
							<Label htmlFor="image-template-pick">
								{t("ui.image.template.pick")}
							</Label>
							<select
								id="image-template-pick"
								value={chosen}
								onChange={(event) => void pick(event.target.value)}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								<option value="">{t("ui.image.template.choose")}</option>
								{templates.map((tpl) => (
									<option key={tpl.image_id} value={tpl.image_id}>
										{tpl.name}
									</option>
								))}
							</select>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="image-template-name">
								{t("ui.image.index.name")}
							</Label>
							<Input
								id="image-template-name"
								value={name}
								maxLength={120}
								onChange={(event) => setName(event.target.value)}
							/>
						</div>
					</div>
					{slots.map((slot) => {
						const id = `image-slot-${slot.name}`;
						const label = (
							<>
								<UntrustedText>{slot.layerName}</UntrustedText>{" "}
								<span className="text-muted-foreground">
									({slot.name}
									{slot.required ? `, ${t("ui.image.template.required")}` : ""})
								</span>
							</>
						);
						return (
							<div key={slot.name} className="flex max-w-md flex-col gap-1">
								<Label htmlFor={id}>{label}</Label>
								{slot.kind === "text" ? (
									<Input
										id={id}
										value={values[slot.name]?.text ?? ""}
										onChange={(event) =>
											setValues({
												...values,
												[slot.name]: { text: event.target.value },
											})
										}
									/>
								) : (
									<select
										id={id}
										value={values[slot.name]?.asset ?? ""}
										onChange={(event) =>
											setValues({
												...values,
												[slot.name]: { asset: event.target.value },
											})
										}
										className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
									>
										<option value="">{t("ui.image.template.no_image")}</option>
										{(library?.images ?? []).map((asset) => (
											<option key={asset.hash} value={asset.hash}>
												{asset.name}
											</option>
										))}
									</select>
								)}
							</div>
						);
					})}
					<div>
						<Button type="submit" disabled={busy || chosen === ""}>
							{t("ui.image.template.create")}
						</Button>
					</div>
				</form>
			)}
			<FailureNote failure={failure} />
			{failure !== null && failure.code === "E_SLOT" && (
				<p className="text-sm text-muted-foreground">
					{t("ui.image.template.slot_hint")}
				</p>
			)}
		</section>
	);
}
