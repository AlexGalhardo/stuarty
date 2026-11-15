import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { BrandKit, TemplateSummary } from "../../editor/api/types";
import { templateFileUrl } from "../../editor/api/urls";

const NAME_MAX = 120;

type Loaded =
	| { state: "loading" }
	| { state: "failed" }
	| { state: "ready"; templates: TemplateSummary[] };

function TemplateRow({
	template,
	kit,
	onFailure,
}: {
	template: TemplateSummary;
	kit: BrandKit | null;
	onFailure(failure: Failure | null): void;
}): ReactNode {
	const t = useT();
	const [open, setOpen] = useState(false);
	const [name, setName] = useState("");
	const [busy, setBusy] = useState(false);
	const [useKit, setUseKit] = useState(false);
	// The template colours the kit also names, and the kit families: the core
	// takes both lists by name and refuses one the kit does not hold.
	const brandColors = Object.keys(template.colors ?? {}).filter(
		(name) => kit?.colors?.[name] !== undefined,
	);
	const brandFonts = Object.keys(kit?.fonts ?? {});
	const required = template.slots.filter((slot) => slot.required).length;

	async function create(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		onFailure(null);
		try {
			const made = await createClient(
				new HttpTransport("/api/v1"),
			).instantiateTemplate(template.template_id, {
				name: name.trim(),
				...(useKit && brandColors.length > 0
					? { brand_colors: brandColors }
					: {}),
				...(useKit && brandFonts.length > 0 ? { brand_fonts: brandFonts } : {}),
			});
			// The editor is another document, so this is a navigation, not an
			// Inertia visit (SDD-012 section 2.2).
			window.location.assign(`/editor/${made.project_id}`);
		} catch (error) {
			onFailure(failureOf(error));
			setBusy(false);
		}
	}

	return (
		<li
			className="flex flex-col gap-3 p-4"
			data-template-id={template.template_id}
		>
			<div className="flex items-center justify-between gap-4">
				<div className="min-w-0">
					<p className="truncate font-medium">
						<UntrustedText>{template.name}</UntrustedText>
					</p>
					{template.description !== "" && (
						<p className="truncate text-xs text-muted-foreground">
							<UntrustedText>{template.description}</UntrustedText>
						</p>
					)}
					<p className="text-xs text-muted-foreground">
						{t("ui.projects.templates.summary", {
							width: template.width,
							height: template.height,
							slots: template.slots.length,
							required,
						})}
					</p>
				</div>
				<div className="flex shrink-0 gap-2">
					<Button
						type="button"
						variant="outline"
						aria-expanded={open}
						onClick={() => setOpen((was) => !was)}
					>
						{t("ui.projects.templates.new_from")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{template.name}</UntrustedText>
						</span>
					</Button>
					<Button asChild variant="outline">
						<a href={templateFileUrl(template.template_id)} download>
							{t("ui.projects.templates.export")}
							<span className="sr-only">
								{" "}
								<UntrustedText>{template.name}</UntrustedText>
							</span>
						</a>
					</Button>
				</div>
			</div>
			{open && (
				<form
					onSubmit={(event) => void create(event)}
					className="flex items-end gap-2"
				>
					<div className="flex flex-1 flex-col gap-1">
						<Label htmlFor={`from-${template.template_id}`}>
							{t("ui.projects.name")}
						</Label>
						<Input
							id={`from-${template.template_id}`}
							required
							maxLength={NAME_MAX}
							autoComplete="off"
							value={name}
							onChange={(event) => setName(event.target.value)}
						/>
					</div>
					{(brandColors.length > 0 || brandFonts.length > 0) && (
						<label className="flex items-center gap-1.5 pb-2 text-sm">
							<input
								type="checkbox"
								checked={useKit}
								className="accent-primary"
								onChange={(event) => setUseKit(event.target.checked)}
							/>
							{t("ui.projects.templates.use_brand")}
						</label>
					)}
					<Button type="submit" disabled={busy || name.trim() === ""}>
						{t("ui.projects.templates.create")}
					</Button>
				</form>
			)}
		</li>
	);
}

// The template entries of the projects page: the list of the templates with
// "New from template" and the template file export, and the import of a file
// by its path on the machine of the service (the operation takes a path, never
// the bytes, SDD-007 section 7.6). SDD-012 gives no layout for these; the
// proposal is recorded in section 2.1.
export default function Templates(): ReactNode {
	const t = useT();
	const [loaded, setLoaded] = useState<Loaded>({ state: "loading" });
	const [failure, setFailure] = useState<Failure | null>(null);
	const [path, setPath] = useState("");
	const [importing, setImporting] = useState(false);
	const [kit, setKit] = useState<BrandKit | null>(null);

	useEffect(() => {
		createClient(new HttpTransport("/api/v1"))
			.getBrandKit()
			.then(setKit, () => undefined);
	}, []);

	const load = useCallback(async (): Promise<void> => {
		try {
			const result = await createClient(
				new HttpTransport("/api/v1"),
			).listTemplates();
			setLoaded({ state: "ready", templates: result.templates });
		} catch {
			setLoaded({ state: "failed" });
		}
	}, []);

	useEffect(() => {
		void load();
	}, [load]);

	async function importFile(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setImporting(true);
		setFailure(null);
		try {
			await createClient(new HttpTransport("/api/v1")).importTemplate(
				path.trim(),
			);
			setPath("");
			await load();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setImporting(false);
		}
	}

	return (
		<section aria-labelledby="template-list">
			<h2 id="template-list" className="text-lg font-semibold">
				{t("ui.projects.templates.title")}
			</h2>
			<FailureNote failure={failure} />
			{loaded.state === "loading" && (
				<p className="mt-4 text-sm text-muted-foreground">
					{t("ui.projects.templates.loading")}
				</p>
			)}
			{loaded.state === "failed" && (
				<p role="alert" className="mt-4 text-sm text-destructive">
					{t("ui.projects.templates.failed")}
				</p>
			)}
			{loaded.state === "ready" &&
				(loaded.templates.length === 0 ? (
					<p className="mt-4 text-sm text-muted-foreground">
						{t("ui.projects.templates.empty")}
					</p>
				) : (
					<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
						{loaded.templates.map((template) => (
							<TemplateRow
								key={template.template_id}
								template={template}
								kit={kit}
								onFailure={setFailure}
							/>
						))}
					</ul>
				))}
			<form
				onSubmit={(event) => void importFile(event)}
				className="mt-4 flex items-end gap-2"
			>
				<div className="flex flex-1 flex-col gap-1">
					<Label htmlFor="template-import-path">
						{t("ui.projects.templates.import_path")}
					</Label>
					<Input
						id="template-import-path"
						autoComplete="off"
						value={path}
						onChange={(event) => setPath(event.target.value)}
					/>
				</div>
				<Button
					type="submit"
					variant="outline"
					disabled={importing || path.trim() === ""}
				>
					{t("ui.projects.templates.import")}
				</Button>
			</form>
		</section>
	);
}
