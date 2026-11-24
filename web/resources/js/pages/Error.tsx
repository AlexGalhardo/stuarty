import { Head, Link } from "@inertiajs/react";
import { ArrowLeft, CircleAlert, House } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";

// The words that tell what to do next, by the kind of error. They are copy only: the page never
// decides what an error means, and the text of the core is shown as it came.
function nextStep(
	status: number,
): "not_found" | "forbidden" | "server" | "other" {
	if (status === 404) return "not_found";
	if (status === 401 || status === 403) return "forbidden";
	if (status >= 500) return "server";
	return "other";
}

// Rendered by the server for errors on Inertia visits (404, 403, 500…).
// message is safe to show: 5xx only ever carry the status text. A page that a
// core error stopped (a project that cannot open) also carries the code and the
// hint of the error (SDD-012 section 4.4), shown verbatim; the next step is
// added under them.
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
}) {
	const t = useT();
	const next = nextStep(status);
	return (
		<>
			<Head title={`${status} ${message}`} />
			<section
				aria-labelledby="error-title"
				className="mx-auto flex w-full max-w-xl flex-col gap-4 rounded-lg border border-border bg-surface p-6 text-text"
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
						<h1
							id="error-title"
							lang="en"
							className="text-2xl font-semibold [overflow-wrap:anywhere]"
						>
							{message}
						</h1>
					</div>
				</div>
				{hint !== undefined && (
					<p className="text-base text-text-muted">
						{t("ui.error.how_to_fix")} <span lang="en">{hint}</span>
					</p>
				)}
				{code !== undefined && (
					<p className="font-mono text-sm text-text-subtle">{code}</p>
				)}
				<div className="border-t border-border pt-4" data-testid="error-next">
					<p className="text-base font-semibold">
						{t(`ui.error.next.${next}.title`)}
					</p>
					<p className="text-sm text-text-muted">
						{t(`ui.error.next.${next}.body`)}
					</p>
				</div>
				<div className="flex flex-wrap gap-2">
					<Button asChild>
						<Link href="/projects">
							<ArrowLeft aria-hidden="true" />
							{t("ui.error.back")}
						</Link>
					</Button>
					<Button asChild variant="outline">
						<Link href="/">
							<House aria-hidden="true" />
							{t("ui.error.home")}
						</Link>
					</Button>
				</div>
			</section>
		</>
	);
}
