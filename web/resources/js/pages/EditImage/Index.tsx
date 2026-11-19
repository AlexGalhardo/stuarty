import { Head, Link } from "@inertiajs/react";
import {
	type FormEvent,
	type ReactNode,
	useEffect,
	useMemo,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { HttpTransport } from "../../editor/api/http";
import { API_BASE, createImageApi } from "../../image-editor/api/imageApi";
import type { ImageSummary } from "../../image-editor/api/types";
import NewFromTemplate from "../../image-editor/templates/NewFromTemplate";

// The sizes a new image starts from: the names of the presets of SDD-021 section 6, and a size of your own.
const PRESETS = ["video_thumbnail", "square_post", "vertical_cover"] as const;
const CUSTOM = "custom";
const NAME_MAX = 120;

type Loaded =
	| { state: "loading" }
	| { state: "failed"; failure: Failure }
	| { state: "ready"; images: ImageSummary[] };

// Edit Image (RF-341, SDD-012 section 24.1): the documents, a new blank image and a new one from a template.
export default function Index(): ReactNode {
	const t = useT();
	const api = useMemo(() => createImageApi(new HttpTransport(API_BASE)), []);
	const [loaded, setLoaded] = useState<Loaded>({ state: "loading" });
	const [name, setName] = useState("");
	const [preset, setPreset] = useState<string>(PRESETS[0]);
	const [width, setWidth] = useState("1280");
	const [height, setHeight] = useState("720");
	// `image.create` with `template`: a template is made here and filled from "New from template".
	const [asTemplate, setAsTemplate] = useState(false);
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);

	useEffect(() => {
		let live = true;
		api
			.list()
			.then((result) => {
				if (live) setLoaded({ state: "ready", images: result.images });
			})
			.catch((error: unknown) => {
				if (live) setLoaded({ state: "failed", failure: failureOf(error) });
			});
		return () => {
			live = false;
		};
	}, [api]);

	async function create(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			const made = await api.create({
				...(name.trim() === "" ? {} : { name: name.trim() }),
				...(preset === CUSTOM
					? { width: Number(width), height: Number(height) }
					: { preset }),
				...(asTemplate ? { template: true } : {}),
			});
			window.location.assign(`/edit-image/${made.image_id}`);
		} catch (error) {
			setFailure(failureOf(error));
			setBusy(false);
		}
	}

	const documents =
		loaded.state === "ready" ? loaded.images.filter((i) => !i.template) : [];
	// The templates have a list of their own, so a person can open one and give it its slots.
	const templates =
		loaded.state === "ready" ? loaded.images.filter((i) => i.template) : [];

	return (
		<>
			<Head title={t("ui.image.index.title")} />
			<div className="mx-auto flex max-w-4xl flex-col gap-8">
				<h1 className="text-2xl font-semibold tracking-tight">
					{t("ui.image.index.title")}
				</h1>

				<section
					aria-labelledby="image-new-title"
					className="flex flex-col gap-3"
				>
					<h2 id="image-new-title" className="text-lg font-medium">
						{t("ui.image.index.new")}
					</h2>
					<form
						onSubmit={(event) => void create(event)}
						className="flex flex-wrap items-end gap-3"
						data-testid="image-new-form"
					>
						<div className="flex flex-col gap-1">
							<Label htmlFor="image-new-name">{t("ui.image.index.name")}</Label>
							<Input
								id="image-new-name"
								value={name}
								maxLength={NAME_MAX}
								onChange={(event) => setName(event.target.value)}
								placeholder={t("ui.image.index.name_placeholder")}
							/>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="image-new-size">{t("ui.image.index.size")}</Label>
							<select
								id="image-new-size"
								value={preset}
								onChange={(event) => setPreset(event.target.value)}
								className="h-8 rounded-lg border border-input bg-background px-2 text-sm"
							>
								{PRESETS.map((p) => (
									<option key={p} value={p}>
										{t(`ui.image.preset.${p}`)}
									</option>
								))}
								<option value={CUSTOM}>{t("ui.image.preset.custom")}</option>
							</select>
						</div>
						{preset === CUSTOM && (
							<>
								<div className="flex w-24 flex-col gap-1">
									<Label htmlFor="image-new-width">
										{t("ui.image.index.width")}
									</Label>
									<Input
										id="image-new-width"
										inputMode="numeric"
										value={width}
										onChange={(event) => setWidth(event.target.value)}
									/>
								</div>
								<div className="flex w-24 flex-col gap-1">
									<Label htmlFor="image-new-height">
										{t("ui.image.index.height")}
									</Label>
									<Input
										id="image-new-height"
										inputMode="numeric"
										value={height}
										onChange={(event) => setHeight(event.target.value)}
									/>
								</div>
							</>
						)}
						<label className="flex h-8 items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={asTemplate}
								onChange={(event) => setAsTemplate(event.target.checked)}
								data-testid="image-new-template"
							/>
							{t("ui.image.index.as_template")}
						</label>
						<Button type="submit" disabled={busy}>
							{t("ui.image.index.create")}
						</Button>
					</form>
					<FailureNote failure={failure} />
				</section>

				<NewFromTemplate api={api} />

				<section
					aria-labelledby="image-list-title"
					className="flex flex-col gap-3"
				>
					<h2 id="image-list-title" className="text-lg font-medium">
						{t("ui.image.index.documents")}
					</h2>
					{loaded.state === "loading" && (
						<p className="text-sm text-muted-foreground">
							{t("ui.image.index.loading")}
						</p>
					)}
					{loaded.state === "failed" && (
						<FailureNote failure={loaded.failure} />
					)}
					{loaded.state === "ready" && documents.length === 0 && (
						<p className="text-sm text-muted-foreground">
							{t("ui.image.index.empty")}
						</p>
					)}
					{documents.length > 0 && (
						<ul className="grid gap-2 sm:grid-cols-2" data-testid="image-list">
							{documents.map((image) => (
								<li key={image.image_id}>
									<Link
										href={`/edit-image/${image.image_id}`}
										className="flex flex-col gap-0.5 rounded-lg border p-3 hover:bg-muted"
									>
										<span className="font-medium">
											<UntrustedText>{image.name}</UntrustedText>
										</span>
										<span className="text-xs text-muted-foreground tabular-nums">
											{t("ui.image.editor.size", {
												w: image.width,
												h: image.height,
											})}
											{" · "}
											{t("ui.image.index.layers", { count: image.layers })}
										</span>
									</Link>
								</li>
							))}
						</ul>
					)}
				</section>

				{templates.length > 0 && (
					<section
						aria-labelledby="image-templates-title"
						className="flex flex-col gap-3"
					>
						<h2 id="image-templates-title" className="text-lg font-medium">
							{t("ui.image.index.templates")}
						</h2>
						<ul
							className="grid gap-2 sm:grid-cols-2"
							data-testid="image-template-list"
						>
							{templates.map((image) => (
								<li key={image.image_id}>
									<Link
										href={`/edit-image/${image.image_id}`}
										className="flex flex-col gap-0.5 rounded-lg border p-3 hover:bg-muted"
									>
										<span className="font-medium">
											<UntrustedText>{image.name}</UntrustedText>
										</span>
										<span className="text-xs text-muted-foreground tabular-nums">
											{t("ui.image.editor.size", {
												w: image.width,
												h: image.height,
											})}
											{" · "}
											{t("ui.image.index.layers", { count: image.layers })}
										</span>
									</Link>
								</li>
							))}
						</ul>
					</section>
				)}
			</div>
		</>
	);
}
