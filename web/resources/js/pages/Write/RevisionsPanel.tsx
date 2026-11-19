import { type ReactNode, useCallback, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
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
import { keyed } from "./keyed";

// The revisions of a text (SDD-019 sections 3 and 11): who saved each, what a model drafted and at what cost, the
// text of one, the difference with the one before it, and the way back. Going back makes a new revision: nothing
// is ever deleted by it.
export default function RevisionsPanel({
	client,
	projectId,
	itemId,
	latest,
	onReverted,
}: {
	client: WritingClient;
	projectId: string;
	itemId: string;
	// The latest revision of the text: when it changes, the list is read again.
	latest: number;
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
	const [diff, setDiff] = useState<{
		from: number;
		to: number;
		hunks: Hunk[];
		coarse: boolean;
	} | null>(null);
	const [busy, setBusy] = useState(false);

	// biome-ignore lint/correctness/useExhaustiveDependencies: `latest` is the trigger of the read
	useEffect(() => {
		let live = true;
		setLoading(true);
		setReading(null);
		setDiff(null);
		client
			.revisions(projectId, itemId)
			.then((result) => {
				if (!live) return;
				setList(result);
				setFailure(null);
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

	async function compare(revision: number): Promise<void> {
		setFailure(null);
		try {
			const got = await client.revisions(projectId, itemId, {
				from: revision - 1,
				to: revision,
			});
			setDiff({
				from: revision - 1,
				to: revision,
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

	return (
		<section
			aria-labelledby="revisions-heading"
			className="flex flex-col gap-2"
			data-testid="revisions-panel"
		>
			<h3 id="revisions-heading" className="font-medium">
				{t("ui.write.revisions.heading")}
			</h3>
			<FailureNote failure={failure} />
			{loading && (
				<p role="status" className="text-sm text-muted-foreground">
					{t("ui.write.revisions.loading")}
				</p>
			)}
			<ul className="flex flex-col gap-2">
				{list?.revisions.map((revision) => (
					<li
						key={revision.revision}
						data-revision={revision.revision}
						className="flex flex-col gap-1 rounded-lg border p-2 text-sm"
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
									onClick={() => void compare(revision.revision)}
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
								className="max-h-64 overflow-auto rounded border bg-muted/40 p-2 whitespace-pre-wrap break-words"
							>
								<UntrustedText>{reading.body}</UntrustedText>
							</pre>
						)}
					</li>
				))}
			</ul>
			{diff !== null && <DiffView diff={diff} />}
		</section>
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

// The difference between two revisions, by paragraph, as the core compared them. The sign and the word say what
// each paragraph is, so the colour is never the only sign.
function DiffView({
	diff,
}: {
	diff: { from: number; to: number; hunks: Hunk[]; coarse: boolean };
}): ReactNode {
	const t = useT();
	const changed = diff.hunks.filter((hunk) => hunk.op !== "equal");
	return (
		<div data-testid="revision-diff" className="flex flex-col gap-1 text-sm">
			<h4 className="font-medium">
				{t("ui.write.revisions.diff_heading", {
					from: String(diff.from),
					to: String(diff.to),
				})}
			</h4>
			{diff.coarse && (
				<p className="text-muted-foreground">
					{t("ui.write.revisions.diff_coarse")}
				</p>
			)}
			{changed.length === 0 && (
				<p className="text-muted-foreground">
					{t("ui.write.revisions.diff_none")}
				</p>
			)}
			{keyed(changed, (h) => `${h.op}:${h.text}`).map(({ key, item: hunk }) => (
				<p
					key={key}
					data-op={hunk.op}
					className={
						hunk.op === "added"
							? "rounded border-l-4 border-green-700 bg-green-50 p-2 dark:bg-green-950"
							: "rounded border-l-4 border-red-700 bg-red-50 p-2 dark:bg-red-950"
					}
				>
					<span className="font-medium">
						{hunk.op === "added" ? "+ " : "− "}
						<span className="sr-only">
							{hunk.op === "added"
								? t("ui.write.revisions.diff_added")
								: t("ui.write.revisions.diff_removed")}
							:{" "}
						</span>
					</span>
					<UntrustedText className="whitespace-pre-wrap break-words">
						{hunk.text}
					</UntrustedText>
				</p>
			))}
		</div>
	);
}
