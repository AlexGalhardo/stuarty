import type { ReactNode } from "react";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { useT } from "@/lib/i18n";
import type { LibraryEntry, LibraryFinding, LibraryKind } from "./client";

// The state of an entry as a word and an icon (SDD-018 section 14, design-system section 7): what the service says,
// never computed here. A skill or a note an agent may not use yet says so in words: that is the line a person reads
// to know what steers the agents.
export function EntryBadges({ entry }: { entry: LibraryEntry }): ReactNode {
	const t = useT();
	return (
		<span className="flex flex-wrap items-center gap-1">
			{entry.state === "invalid" && (
				<StatusBadge status="danger">
					{t("ui.library.badge.invalid")}
				</StatusBadge>
			)}
			{entry.kind === "skill" && entry.origin !== undefined && (
				<StatusBadge status="neutral">
					{t(`ui.library.origin.${entry.origin}`)}
				</StatusBadge>
			)}
			{entry.kind === "skill" && entry.state === "ok" && (
				<StatusBadge status={entry.reviewed ? "success" : "waiting"}>
					{entry.reviewed
						? t("ui.library.badge.reviewed")
						: t("ui.library.badge.unreviewed")}
				</StatusBadge>
			)}
			{entry.kind === "skill" && entry.has_scripts === true && (
				<StatusBadge status="neutral">
					{t("ui.library.badge.scripts")}
				</StatusBadge>
			)}
			{entry.kind === "memory" && entry.state === "ok" && (
				<StatusBadge status={entry.kept ? "success" : "waiting"}>
					{entry.kept
						? t("ui.library.badge.kept")
						: t("ui.library.badge.pending")}
				</StatusBadge>
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
			<p className="text-sm text-text-muted">{t("ui.library.lint.none")}</p>
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
				className="max-h-96 overflow-auto rounded-lg border bg-surface-sunken p-3 text-sm whitespace-pre-wrap break-words"
			>
				<UntrustedText>{text}</UntrustedText>
			</pre>
		</section>
	);
}

export interface EntryGroup {
	key: string;
	label: string | null;
	entries: readonly LibraryEntry[];
}

// Memory shows the notes that wait for a person first: which are kept is the service's word, this only sets the
// order and the headings. The other lists keep the order of the service, with no heading.
export function groupEntries(
	kind: LibraryKind,
	rows: readonly LibraryEntry[],
	labels: { pending: string; kept: string },
): EntryGroup[] {
	if (kind !== "memory") return [{ key: "all", label: null, entries: rows }];
	const waits = (e: LibraryEntry): boolean => e.state === "ok" && !e.kept;
	return [
		{ key: "pending", label: labels.pending, entries: rows.filter(waits) },
		{ key: "kept", label: labels.kept, entries: rows.filter((e) => !waits(e)) },
	].filter((g) => g.entries.length > 0);
}
