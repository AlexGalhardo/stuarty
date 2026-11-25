import { type ReactNode, useCallback, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { LoadingState } from "@/components/LoadingState";
import { OptionSelect } from "@/components/OptionSelect";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { formatMoney } from "../../dashboard/approval/money";
import type {
	Hunk,
	TextRevision,
	TextRevisionsResult,
	WritingClient,
} from "./client";
import { changedCount, sidesOf } from "./compare";
import { keyed } from "./keyed";
import Panel from "./Panel";

interface Comparison {
	from: number;
	to: number;
	hunks: Hunk[];
	coarse: boolean;
}

// The review of a text (SDD-019 sections 3 and 11): who saved each revision, what a model drafted and at what cost,
// the text of one, two revisions side by side with what changed, and the way back. Going back makes a new revision:
// nothing is ever deleted by it, so a person can compare, restore and still return to what they had.
export default function RevisionsPanel({
	client,
	projectId,
	itemId,
	latest,
	autoCompare = false,
	onReverted,
}: {
	client: WritingClient;
	projectId: string;
	itemId: string;
	// The latest revision of the text: when it changes, the list is read again.
	latest: number;
	// The review view shows what changed between the newest two revisions without a click.
	autoCompare?: boolean;
	onReverted: (message: string) => void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [list, setList] = useState<TextRevisionsResult | null>(null);
	const [loading, setLoading] = useState(true);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [reading, setReading] = useState<{
		revision: number;
		body: string;
	} | null>(null);
	const [comparison, setComparison] = useState<Comparison | null>(null);
	const [from, setFrom] = useState("");
	const [to, setTo] = useState("");
	const [busy, setBusy] = useState(false);

	// biome-ignore lint/correctness/useExhaustiveDependencies: `latest` is the trigger of the read, `autoCompare` is fixed for a panel
	useEffect(() => {
		let live = true;
		setLoading(true);
		setReading(null);
		setComparison(null);
		client
			.revisions(projectId, itemId)
			.then((result) => {
				if (!live) return;
				setList(result);
				setFailure(null);
				// The newest two revisions are what a person usually wants to see first.
				const numbers = result.revisions.map((r) => r.revision);
				setTo(String(numbers[0] ?? ""));
				setFrom(String(numbers[1] ?? ""));
				const older = numbers[1];
				const newer = numbers[0];
				if (autoCompare && older !== undefined && newer !== undefined) {
					void client
						.revisions(projectId, itemId, { from: older, to: newer })
						.then((got) => {
							if (!live) return;
							setComparison({
								from: older,
								to: newer,
								hunks: got.diff?.hunks ?? [],
								coarse: got.diff?.coarse ?? false,
							});
						})
						.catch(() => undefined);
				}
			})
			.catch((error: unknown) => {
				if (live) setFailure(failureOf(error));
			})
			.finally(() => {
				if (live) setLoading(false);
			});
		return () => {
			live = false;
		};
	}, [client, projectId, itemId, latest]);

	const read = useCallback(
		async (revision: number): Promise<void> => {
			setFailure(null);
			if (reading?.revision === revision) {
				setReading(null);
				return;
			}
			try {
				const got = await client.get(projectId, itemId, revision);
				setReading({ revision, body: got.body });
			} catch (error) {
				setFailure(failureOf(error));
			}
		},
		[client, projectId, itemId, reading],
	);

	async function compare(older: number, newer: number): Promise<void> {
		setFailure(null);
		try {
			const got = await client.revisions(projectId, itemId, {
				from: older,
				to: newer,
			});
			setComparison({
				from: older,
				to: newer,
				hunks: got.diff?.hunks ?? [],
				coarse: got.diff?.coarse ?? false,
			});
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	async function back(revision: number): Promise<void> {
		setBusy(true);
		setFailure(null);
		try {
			const done = await client.revert(projectId, itemId, revision);
			onReverted(
				t("ui.write.revisions.went_back", {
					from: String(revision),
					revision: String(done.revision.revision),
				}),
			);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	const revisions = list?.revisions ?? [];
	const canCompare = revisions.length >= 2 && from !== "" && to !== "";

	return (
		<Panel
			title={t("ui.write.revisions.heading")}
			description={t("ui.write.revisions.intro")}
			testId="revisions-panel"
			headingId="revisions-heading"
		>
			<FailureNote failure={failure} />
			{loading && <LoadingState message={t("ui.write.revisions.loading")} />}
			{!loading && list !== null && revisions.length < 2 && (
				<p className="text-sm text-muted-foreground" data-testid="compare-need">
					{t("ui.write.revisions.compare_need")}
				</p>
			)}
			{revisions.length >= 2 && (
				<form
					className="flex flex-wrap items-end gap-3"
					aria-label={t("ui.write.revisions.compare_form")}
					onSubmit={(event) => {
						event.preventDefault();
						void compare(Number(from), Number(to));
					}}
				>
					<RevisionSelect
						label={t("ui.write.revisions.compare_from")}
						value={from}
						onChange={setFrom}
						revisions={revisions}
					/>
					<RevisionSelect
						label={t("ui.write.revisions.compare_to")}
						value={to}
						onChange={setTo}
						revisions={revisions}
					/>
					<Button type="submit" variant="outline" disabled={!canCompare}>
						{t("ui.write.revisions.compare_go")}
					</Button>
				</form>
			)}
			{comparison !== null && (
				<CompareView
					comparison={comparison}
					latest={latest}
					busy={busy}
					onRestore={(revision) => void back(revision)}
				/>
			)}
			<ul className="flex flex-col gap-2">
				{revisions.map((revision) => (
					<li
						key={revision.revision}
						data-revision={revision.revision}
						className="flex flex-col gap-1 rounded-lg border border-border p-3 text-sm"
					>
						<p>
							<UntrustedText>
								{t("ui.write.revisions.line", {
									revision: String(revision.revision),
									source: t(`ui.write.revisions.source.${revision.source}`),
									actor: revision.actor,
								})}
							</UntrustedText>
							{" · "}
							<span className="text-muted-foreground">
								{formatDateTime(revision.created_at, locale)}
							</span>
						</p>
						{revision.reverted_to !== undefined && (
							<p className="text-muted-foreground">
								{t("ui.write.revisions.reverted_to", {
									revision: String(revision.reverted_to),
								})}
							</p>
						)}
						<ModelLine revision={revision} />
						<div className="flex flex-wrap gap-2">
							<Button
								type="button"
								size="sm"
								variant="outline"
								onClick={() => void read(revision.revision)}
							>
								{t("ui.write.revisions.read")}
							</Button>
							{revision.revision > 1 && (
								<Button
									type="button"
									size="sm"
									variant="outline"
									onClick={() => {
										setFrom(String(revision.revision - 1));
										setTo(String(revision.revision));
										void compare(revision.revision - 1, revision.revision);
									}}
								>
									{t("ui.write.revisions.compare")}
								</Button>
							)}
							{revision.revision !== latest && (
								<Button
									type="button"
									size="sm"
									variant="outline"
									disabled={busy}
									onClick={() => void back(revision.revision)}
								>
									{t("ui.write.revisions.back")}
								</Button>
							)}
						</div>
						{reading?.revision === revision.revision && (
							<pre
								// biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable text must be reachable by keyboard
								tabIndex={0}
								data-testid="revision-body"
								className="max-h-64 overflow-auto rounded border border-border bg-surface-hover p-2 whitespace-pre-wrap break-words"
							>
								<UntrustedText>{reading.body}</UntrustedText>
							</pre>
						)}
					</li>
				))}
			</ul>
		</Panel>
	);
}

function RevisionSelect({
	label,
	value,
	onChange,
	revisions,
}: {
	label: string;
	value: string;
	onChange: (value: string) => void;
	revisions: readonly TextRevision[];
}): ReactNode {
	const t = useT();
	return (
		<div className="flex min-w-0 max-w-full flex-col gap-1">
			<span className="text-sm font-medium text-text">{label}</span>
			<OptionSelect
				label={label}
				value={value}
				onChange={onChange}
				options={revisions.map((revision) => ({
					value: String(revision.revision),
					label: t("ui.write.revisions.option", {
						revision: String(revision.revision),
						source: t(`ui.write.revisions.source.${revision.source}`),
					}),
				}))}
			/>
		</div>
	);
}

function ModelLine({ revision }: { revision: TextRevision }): ReactNode {
	const t = useT();
	const locale = useLocale();
	if (revision.provider === undefined) return null;
	return (
		<p className="text-muted-foreground">
			<UntrustedText>
				{t("ui.write.revisions.model_line", {
					provider: revision.provider,
					model: revision.model ?? "",
					cost:
						revision.cost === undefined
							? ""
							: formatMoney(revision.cost, locale),
				})}
			</UntrustedText>
		</p>
	);
}

// Two revisions side by side, the older on the left and the newer on the right, stacked at phone width. The core
// compared them by paragraph: the sign and the word say what each paragraph is, so the colour is never the only
// sign. A revision that is not the latest can be restored from here; that stores it as a new revision.
export function CompareView({
	comparison,
	latest,
	busy,
	onRestore,
}: {
	comparison: Comparison;
	latest: number;
	busy: boolean;
	onRestore: (revision: number) => void;
}): ReactNode {
	const t = useT();
	const sides = sidesOf(comparison.hunks);
	const changed = changedCount(comparison.hunks);
	return (
		<section
			data-testid="revision-diff"
			className="flex flex-col gap-3 text-sm"
			aria-labelledby="compare-heading"
		>
			<h3 id="compare-heading" className="text-base font-semibold">
				{t("ui.write.revisions.diff_heading", {
					from: String(comparison.from),
					to: String(comparison.to),
				})}
			</h3>
			{comparison.coarse && (
				<p className="text-muted-foreground">
					{t("ui.write.revisions.diff_coarse")}
				</p>
			)}
			{changed === 0 && (
				<p className="text-muted-foreground">
					{t("ui.write.revisions.diff_none")}
				</p>
			)}
			<div className="grid min-w-0 gap-4 md:grid-cols-2">
				<Side
					column="from"
					revision={comparison.from}
					hunks={sides.from}
					latest={latest}
					busy={busy}
					onRestore={onRestore}
				/>
				<Side
					column="to"
					revision={comparison.to}
					hunks={sides.to}
					latest={latest}
					busy={busy}
					onRestore={onRestore}
				/>
			</div>
			<p className="text-muted-foreground">
				{t("ui.write.revisions.restore_note")}
			</p>
		</section>
	);
}

function Side({
	column,
	revision,
	hunks,
	latest,
	busy,
	onRestore,
}: {
	column: "from" | "to";
	revision: number;
	hunks: Hunk[];
	latest: number;
	busy: boolean;
	onRestore: (revision: number) => void;
}): ReactNode {
	const t = useT();
	return (
		<section
			data-column={column}
			aria-label={t("ui.write.revisions.column", {
				revision: String(revision),
			})}
			className="flex min-w-0 flex-col gap-2 rounded-lg border border-border p-3"
		>
			<div className="flex flex-wrap items-center justify-between gap-2">
				<h4 className="font-medium">
					{t("ui.write.revisions.column", { revision: String(revision) })}
				</h4>
				{revision !== latest && (
					<Button
						type="button"
						size="sm"
						variant="outline"
						disabled={busy}
						onClick={() => onRestore(revision)}
					>
						{t("ui.write.revisions.restore", { revision: String(revision) })}
					</Button>
				)}
			</div>
			{hunks.length === 0 && (
				<p className="text-muted-foreground">
					{t("ui.write.revisions.column_empty")}
				</p>
			)}
			{keyed(hunks, (h) => `${h.op}:${h.text}`).map(({ key, item: hunk }) => (
				<p
					key={key}
					data-op={hunk.op}
					className={
						hunk.op === "added"
							? "rounded border-l-4 border-green-700 bg-green-50 p-2 dark:bg-green-950"
							: hunk.op === "removed"
								? "rounded border-l-4 border-red-700 bg-red-50 p-2 dark:bg-red-950"
								: "rounded border-l-4 border-transparent p-2"
					}
				>
					{hunk.op !== "equal" && (
						<span className="font-medium">
							{hunk.op === "added" ? "+ " : "− "}
							<span className="sr-only">
								{hunk.op === "added"
									? t("ui.write.revisions.diff_added")
									: t("ui.write.revisions.diff_removed")}
								:{" "}
							</span>
						</span>
					)}
					<UntrustedText className="whitespace-pre-wrap break-words">
						{hunk.text}
					</UntrustedText>
				</p>
			))}
		</section>
	);
}
