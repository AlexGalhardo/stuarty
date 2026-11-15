import { Head, Link } from "@inertiajs/react";

import { useT } from "@/lib/i18n";

// Rendered by the server for errors on Inertia visits (404, 403, 500…).
// message is safe to show: 5xx only ever carry the status text. A page that a
// core error stopped (a project that cannot open) also carries the code and the
// hint of the error (SDD-012 section 4.4).
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
	return (
		<>
			<Head title={`${status} ${message}`} />
			<section className="py-12 text-center">
				<p className="text-sm font-semibold text-muted-foreground">{status}</p>
				<h1 className="mt-2 text-3xl font-semibold tracking-tight">
					{message}
				</h1>
				{hint !== undefined && (
					<p className="mt-2 text-muted-foreground">
						{t("ui.error.how_to_fix")} {hint}
					</p>
				)}
				{code !== undefined && (
					<p className="mt-2 font-mono text-xs text-muted-foreground">{code}</p>
				)}
				<Link
					href="/projects"
					className="mt-6 inline-block underline underline-offset-4 hover:text-muted-foreground"
				>
					{t("ui.error.back")}
				</Link>
			</section>
		</>
	);
}
