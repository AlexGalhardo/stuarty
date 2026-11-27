import { Head, Link } from "@inertiajs/react";
import { ArrowLeft, CircleAlert, Trash2 } from "lucide-react";
import type { ReactNode } from "react";

import { Wordmark } from "@/components/AuthCard";
import { NumberedStep } from "@/components/NumberedStep";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import { localizeError } from "@/lib/localizeError";

type Kind = "not_found" | "forbidden" | "server" | "generic";

// The words that tell what to do next, by the kind of error. They are copy only: the page never decides what an
// error means. The message the server sent is English and technical, so it is not the title: it stays under
// "Technical details", as text, for a person who wants to report it.
function kindOf(status: number): Kind {
	if (status === 404) return "not_found";
	if (status === 401 || status === 403) return "forbidden";
	if (status >= 500) return "server";
	return "generic";
}

// Rendered by the server for errors on Inertia visits (404, 403, 500...). It is a screen of its own, centred, with
// no sidebar (the final design). A page that a core error stopped also carries the code and the hint of the error
// (SDD-012 section 4.4); they are shown verbatim under the technical details.
export default function ErrorPage({
	status,
	message,
	code,
	hint,
}: {
	status: number;
	message: string;
	code?: string;
	hint?: string;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const kind = kindOf(status);
	const localized =
		code !== undefined
			? localizeError(t, { code, message, hint }, { locale })
			: null;
	return (
		<>
			<Head title={`${status} ${t(`ui.error.title.${kind}`)}`} />
			<div className="mx-auto flex w-full max-w-xl flex-col gap-4">
				<Wordmark className="justify-start" />
				<section
					aria-labelledby="error-title"
					className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-5 text-text"
				>
					<div className="flex items-center gap-4">
						<span
							aria-hidden="true"
							className="flex size-12 shrink-0 items-center justify-center rounded-lg bg-danger-soft text-danger-text"
						>
							<CircleAlert className="size-6" />
						</span>
						<div className="min-w-0">
							<p className="text-sm font-semibold text-danger-text">
								{t("ui.error.label", { status: String(status) })}
							</p>
							<h1 id="error-title" className="text-2xl font-semibold">
								{t(`ui.error.title.${kind}`)}
							</h1>
						</div>
					</div>
					<p className="text-base text-text-muted">
						{t(`ui.error.lead.${kind}`)}
					</p>
					{localized?.localized && (
						<div className="rounded-md border border-border bg-surface-sunken px-3 py-2 text-sm text-text">
							<p className="font-medium">{localized.headline}</p>
							<p className="mt-0.5 text-text-muted">{localized.message}</p>
							{localized.hint !== undefined && (
								<p className="mt-0.5 text-text-muted">{localized.hint}</p>
							)}
						</div>
					)}
					<div data-testid="error-next">
						<p className="mb-2 text-sm font-semibold">
							{t("ui.error.how_to_fix")}
						</p>
						<ol className="flex flex-col gap-3">
							<NumberedStep
								number={1}
								title={t(`ui.error.steps.${kind}.one.title`)}
								description={t(`ui.error.steps.${kind}.one.body`)}
							/>
							<NumberedStep
								number={2}
								title={t(`ui.error.steps.${kind}.two.title`)}
								description={t(`ui.error.steps.${kind}.two.body`)}
							/>
						</ol>
					</div>
					<div className="flex flex-wrap gap-2">
						<Button asChild size="lg">
							<Link href="/projects">
								<ArrowLeft aria-hidden="true" />
								{t("ui.error.back")}
							</Link>
						</Button>
						<Button asChild size="lg" variant="outline">
							{kind === "not_found" ? (
								<Link href="/projects#project-trash">
									<Trash2 aria-hidden="true" />
									{t("ui.error.trash")}
								</Link>
							) : (
								<Link href="/">{t("ui.error.home")}</Link>
							)}
						</Button>
					</div>
					<details className="border-t border-border pt-3 text-sm text-text-muted">
						<summary className="cursor-pointer font-medium">
							{t("ui.error.details")}
						</summary>
						<p lang="en" className="mt-2 [overflow-wrap:anywhere]">
							<UntrustedText>{message}</UntrustedText>
						</p>
						{hint !== undefined && (
							<p lang="en" className="mt-1 [overflow-wrap:anywhere]">
								<UntrustedText>{hint}</UntrustedText>
							</p>
						)}
						{code !== undefined && (
							<p className="mt-1 font-mono text-text-subtle">{code}</p>
						)}
					</details>
				</section>
			</div>
		</>
	);
}
