import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { useT } from "@/lib/i18n";
import type { LibraryEntry, LibraryFinding } from "./client";

// The state of an entry as small badges (SDD-018 section 14): what the service says, never computed here.
export function EntryBadges({ entry }: { entry: LibraryEntry }): ReactNode {
	const t = useT();
	return (
		<span className="flex flex-wrap items-center gap-1">
			{entry.state === "invalid" && (
				<Badge variant="destructive">{t("ui.library.badge.invalid")}</Badge>
			)}
			{entry.kind === "skill" && entry.origin !== undefined && (
				<Badge variant="outline">
					{t(`ui.library.origin.${entry.origin}`)}
				</Badge>
			)}
			{entry.kind === "skill" && entry.state === "ok" && (
				<Badge variant={entry.reviewed ? "secondary" : "destructive"}>
					{entry.reviewed
						? t("ui.library.badge.reviewed")
						: t("ui.library.badge.unreviewed")}
				</Badge>
			)}
			{entry.kind === "skill" && entry.has_scripts === true && (
				<Badge variant="outline">{t("ui.library.badge.scripts")}</Badge>
			)}
			{entry.kind === "memory" && entry.state === "ok" && (
				<Badge variant={entry.kept ? "secondary" : "destructive"}>
					{entry.kept
						? t("ui.library.badge.kept")
						: t("ui.library.badge.pending")}
				</Badge>
			)}
		</span>
	);
}

// The findings of the skill lint, as the service listed them.
export function Findings({
	findings,
}: {
	findings: readonly LibraryFinding[];
}): ReactNode {
	const t = useT();
	if (findings.length === 0) {
		return (
			<p className="text-sm text-muted-foreground">
				{t("ui.library.lint.none")}
			</p>
		);
	}
	return (
		<ul className="flex flex-col gap-1 text-sm">
			{findings.map((finding) => (
				<li
					key={`${finding.code}:${finding.path ?? ""}:${finding.line ?? 0}`}
					data-severity={finding.severity}
					className="flex flex-wrap gap-x-2"
				>
					<Badge
						variant={finding.severity === "error" ? "destructive" : "outline"}
					>
						{finding.severity === "error"
							? t("ui.library.lint.error")
							: t("ui.library.lint.warning")}
					</Badge>
					<code>{finding.code}</code>
					{finding.path !== undefined && (
						<span>
							<UntrustedText>
								{finding.line === undefined
									? finding.path
									: `${finding.path}:${finding.line}`}
							</UntrustedText>
						</span>
					)}
					<span lang="en">
						<UntrustedText>{finding.message}</UntrustedText>
					</span>
				</li>
			))}
		</ul>
	);
}

// The text of an entry exactly as the file has it, isolated like every text that came from a file.
export function FileText({
	text,
	label,
}: {
	text: string;
	label: string;
}): ReactNode {
	return (
		<section aria-label={label}>
			<pre
				// biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable text must be reachable by keyboard
				tabIndex={0}
				className="max-h-96 overflow-auto rounded-lg border bg-muted/40 p-3 text-sm whitespace-pre-wrap break-words"
			>
				<UntrustedText>{text}</UntrustedText>
			</pre>
		</section>
	);
}
