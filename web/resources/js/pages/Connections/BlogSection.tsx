import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ConnectionCreateRequest } from "../../editor/api/types";

// The destinations of a blog post (SDD-020 part 5): files for a static site, WordPress, a webhook of the person's own
// automation, and the read API of this server. A destination is a connection of one of these kinds; its settings are
// one JSON document that the service keeps in the vault. The page writes the document from the fields and sends it:
// the service validates every member (the folder, the signing secret, the address rule of a webhook).

export const BLOG_KINDS = [
	"blog_files",
	"blog_wordpress",
	"blog_webhook",
	"blog_webhook_local",
	"blog_api",
] as const;
export type BlogKind = (typeof BLOG_KINDS)[number];

export const TEMPLATES = ["astro", "hugo", "jekyll", "custom"] as const;

// A connection that opens no connection has the placeholder endpoint no request is ever made to (SDD-020 section 42).
const PLACEHOLDER = "http://localhost";

export interface BlogForm {
	kind: BlogKind;
	label: string;
	folder: string;
	template: (typeof TEMPLATES)[number];
	frontMatter: string;
	zip: boolean;
	site: string;
	user: string;
	applicationPassword: string;
	hook: string;
	signingSecret: string;
	mediaBase: string;
}

export const EMPTY_FORM: BlogForm = {
	kind: "blog_files",
	label: "",
	folder: "",
	template: "astro",
	frontMatter: "",
	zip: false,
	site: "",
	user: "",
	applicationPassword: "",
	hook: "",
	signingSecret: "",
	mediaBase: "",
};

// blogRequest writes the request of a destination from the fields of its form: the endpoint and the secret document.
export function blogRequest(form: BlogForm): ConnectionCreateRequest {
	const base = { kind: form.kind, label: form.label.trim() };
	switch (form.kind) {
		case "blog_files":
			return {
				...base,
				endpoint: PLACEHOLDER,
				accepts: ["text", "image", "metadata"],
				secret: JSON.stringify({
					folder: form.folder.trim(),
					template: form.template,
					...(form.template === "custom"
						? { front_matter: form.frontMatter }
						: {}),
					...(form.zip ? { zip: true } : {}),
				}),
			};
		case "blog_wordpress":
			return {
				...base,
				endpoint: form.site.trim(),
				accepts: ["text", "image", "metadata"],
				secret: JSON.stringify({
					user: form.user.trim(),
					application_password: form.applicationPassword,
				}),
			};
		case "blog_webhook":
		case "blog_webhook_local":
			return {
				...base,
				endpoint: form.hook.trim(),
				accepts: ["text", "metadata"],
				secret: JSON.stringify({
					signing_secret: form.signingSecret,
					...(form.mediaBase.trim() === ""
						? {}
						: { media_base_url: form.mediaBase.trim() }),
				}),
			};
		case "blog_api":
			return {
				...base,
				endpoint: PLACEHOLDER,
				accepts: ["text", "image", "metadata"],
				secret: "{}",
			};
	}
}

function ready(form: BlogForm): boolean {
	if (form.label.trim() === "") return false;
	switch (form.kind) {
		case "blog_files":
			return (
				form.folder.trim() !== "" &&
				(form.template !== "custom" || form.frontMatter.trim() !== "")
			);
		case "blog_wordpress":
			return (
				form.site.trim() !== "" &&
				form.user.trim() !== "" &&
				form.applicationPassword !== ""
			);
		case "blog_webhook":
		case "blog_webhook_local":
			return form.hook.trim() !== "" && form.signingSecret !== "";
		case "blog_api":
			return true;
	}
}

const SELECT = "h-8 rounded-lg border border-input bg-background px-2 text-sm";

export default function BlogSection({
	onChanged,
}: {
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const [form, setForm] = useState<BlogForm>(EMPTY_FORM);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [notice, setNotice] = useState<string | null>(null);
	const [busy, setBusy] = useState(false);

	const set = <K extends keyof BlogForm>(key: K, value: BlogForm[K]): void =>
		setForm((was) => ({ ...was, [key]: value }));

	async function create(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		setNotice(null);
		try {
			await createClient(new HttpTransport("/api/v1")).createConnection(
				blogRequest(form),
			);
			setForm({ ...EMPTY_FORM, kind: form.kind });
			setNotice(t("ui.connections.blog.created"));
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<section aria-labelledby="connections-blog" data-testid="blog-section">
			<h2 id="connections-blog" className="text-lg font-semibold">
				{t("ui.connections.blog.title")}
			</h2>
			<p className="mt-1 text-sm text-muted-foreground">
				{t("ui.connections.blog.intro")}
			</p>
			<form
				onSubmit={(event) => void create(event)}
				className="mt-4 flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
				aria-label={t("ui.connections.blog.form")}
			>
				<div className="grid gap-4 sm:grid-cols-2">
					<div className="flex flex-col gap-2">
						<Label htmlFor="blog-kind">{t("ui.connections.blog.kind")}</Label>
						<select
							id="blog-kind"
							className={SELECT}
							value={form.kind}
							onChange={(event) => set("kind", event.target.value as BlogKind)}
						>
							{BLOG_KINDS.map((k) => (
								<option key={k} value={k}>
									{t(`ui.connections.blog.kinds.${k}`)}
								</option>
							))}
						</select>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="blog-label">{t("ui.connections.label")}</Label>
						<Input
							id="blog-label"
							required
							maxLength={60}
							autoComplete="off"
							value={form.label}
							onChange={(event) => set("label", event.target.value)}
						/>
					</div>
				</div>
				<p className="text-xs text-muted-foreground" data-testid="blog-about">
					{t(`ui.connections.blog.about.${form.kind}`)}
				</p>
				{form.kind === "blog_files" && (
					<>
						<div className="flex flex-col gap-2">
							<Label htmlFor="blog-folder">
								{t("ui.connections.blog.folder")}
							</Label>
							<Input
								id="blog-folder"
								autoComplete="off"
								value={form.folder}
								onChange={(event) => set("folder", event.target.value)}
							/>
							<p className="text-xs text-muted-foreground">
								{t("ui.connections.blog.folder_hint")}
							</p>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="blog-template">
								{t("ui.connections.blog.template")}
							</Label>
							<select
								id="blog-template"
								className={SELECT}
								value={form.template}
								onChange={(event) =>
									set(
										"template",
										event.target.value as (typeof TEMPLATES)[number],
									)
								}
							>
								{TEMPLATES.map((name) => (
									<option key={name} value={name}>
										{t(`ui.connections.blog.templates.${name}`)}
									</option>
								))}
							</select>
						</div>
						{form.template === "custom" && (
							<div className="flex flex-col gap-2">
								<Label htmlFor="blog-front-matter">
									{t("ui.connections.blog.front_matter")}
								</Label>
								<Textarea
									id="blog-front-matter"
									rows={6}
									className="font-mono"
									value={form.frontMatter}
									onChange={(event) => set("frontMatter", event.target.value)}
								/>
								<p className="text-xs text-muted-foreground">
									{t("ui.connections.blog.front_matter_hint")}
								</p>
							</div>
						)}
						<label className="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								className="accent-primary"
								checked={form.zip}
								onChange={(event) => set("zip", event.target.checked)}
							/>
							{t("ui.connections.blog.zip")}
						</label>
					</>
				)}
				{form.kind === "blog_wordpress" && (
					<>
						<div className="flex flex-col gap-2">
							<Label htmlFor="blog-site">{t("ui.connections.blog.site")}</Label>
							<Input
								id="blog-site"
								type="url"
								autoComplete="off"
								value={form.site}
								onChange={(event) => set("site", event.target.value)}
							/>
						</div>
						<div className="grid gap-4 sm:grid-cols-2">
							<div className="flex flex-col gap-2">
								<Label htmlFor="blog-user">
									{t("ui.connections.blog.user")}
								</Label>
								<Input
									id="blog-user"
									autoComplete="off"
									value={form.user}
									onChange={(event) => set("user", event.target.value)}
								/>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="blog-app-password">
									{t("ui.connections.blog.application_password")}
								</Label>
								<Input
									id="blog-app-password"
									type="password"
									autoComplete="off"
									value={form.applicationPassword}
									onChange={(event) =>
										set("applicationPassword", event.target.value)
									}
								/>
							</div>
						</div>
						<p className="text-xs text-muted-foreground">
							{t("ui.connections.blog.wordpress_hint")}
						</p>
					</>
				)}
				{(form.kind === "blog_webhook" ||
					form.kind === "blog_webhook_local") && (
					<>
						<div className="flex flex-col gap-2">
							<Label htmlFor="blog-hook">{t("ui.connections.blog.hook")}</Label>
							<Input
								id="blog-hook"
								type="url"
								autoComplete="off"
								value={form.hook}
								onChange={(event) => set("hook", event.target.value)}
							/>
						</div>
						<div className="grid gap-4 sm:grid-cols-2">
							<div className="flex flex-col gap-2">
								<Label htmlFor="blog-signing">
									{t("ui.connections.blog.signing_secret")}
								</Label>
								<Input
									id="blog-signing"
									type="password"
									autoComplete="off"
									value={form.signingSecret}
									onChange={(event) => set("signingSecret", event.target.value)}
								/>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="blog-media-base">
									{t("ui.connections.blog.media_base")}
								</Label>
								<Input
									id="blog-media-base"
									type="url"
									autoComplete="off"
									value={form.mediaBase}
									onChange={(event) => set("mediaBase", event.target.value)}
								/>
							</div>
						</div>
						<p className="text-xs text-muted-foreground">
							{t("ui.connections.blog.signing_hint")}
						</p>
					</>
				)}
				<FailureNote failure={failure} />
				<div role="status" aria-live="polite" className="text-sm">
					{notice}
				</div>
				<div>
					<Button type="submit" disabled={busy || !ready(form)}>
						{t("ui.connections.blog.add")}
					</Button>
				</div>
			</form>
		</section>
	);
}
