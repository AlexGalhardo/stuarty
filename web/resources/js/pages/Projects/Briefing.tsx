import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { useUnsavedGuard } from "@/lib/unsaved";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	BriefingQuestion,
	BriefingQuestionsResult,
} from "../../editor/api/types";

// What the briefing already holds, by question id: the answers of a brief the briefing wrote.
export function savedAnswers(
	briefing: BriefingQuestionsResult,
): Record<string, string> {
	const out: Record<string, string> = {};
	for (const a of briefing.answers ?? []) out[a.id] = a.value;
	return out;
}

// The words of a question are the page's translation by question id. A text and an image have their own words
// (a question of the same id says "for whom" of a post, not of a video), so the set the service served picks the
// catalogue; the questions themselves are always the service's.
export function questionKey(set: string, id: string): string {
	if (set === "text") return `ui.projects.briefing.text.q.${id}`;
	if (set === "image") return `ui.projects.briefing.image.q.${id}`;
	return `ui.projects.briefing.q.${id}`;
}

export function optionKey(set: string, id: string, option: string): string {
	if (set === "text") return `ui.projects.briefing.text.option.${id}.${option}`;
	if (set === "image")
		return `ui.projects.briefing.image.option.${id}.${option}`;
	return `ui.projects.briefing.option.${id}.${option}`;
}

function introKey(set: string): string {
	if (set === "text") return "ui.projects.briefing.text.intro";
	if (set === "image") return "ui.projects.briefing.image.intro";
	return "ui.projects.briefing.intro";
}

function Field({
	q,
	set,
	value,
	onChange,
}: {
	q: BriefingQuestion;
	set: string;
	value: string;
	onChange(value: string): void;
}): ReactNode {
	const t = useT();
	const id = `briefing-${q.id}`;
	const need = t(
		q.required
			? "ui.projects.briefing.required"
			: "ui.projects.briefing.optional",
	);

	let control: ReactNode;
	switch (q.kind) {
		case "choice":
			control = (
				<select
					id={id}
					value={value}
					required={q.required}
					onChange={(event) => onChange(event.target.value)}
					className="h-8 w-fit rounded-lg border border-input bg-background px-2 text-sm"
				>
					<option value="">{t("ui.projects.briefing.choose")}</option>
					{(q.options ?? []).map((o) => (
						<option key={o} value={o}>
							{t(optionKey(set, q.id, o))}
						</option>
					))}
				</select>
			);
			break;
		case "seconds":
		case "words":
			control = (
				<Input
					id={id}
					type="number"
					inputMode="numeric"
					min={q.min}
					max={q.max}
					step={1}
					value={value}
					required={q.required}
					aria-describedby={`${id}-hint`}
					className="w-32"
					onChange={(event) => onChange(event.target.value)}
				/>
			);
			break;
		case "date":
			control = (
				<Input
					id={id}
					type="date"
					value={value}
					required={q.required}
					className="w-fit"
					onChange={(event) => onChange(event.target.value)}
				/>
			);
			break;
		default:
			control = (
				<Textarea
					id={id}
					value={value}
					maxLength={q.max_chars}
					required={q.required}
					rows={2}
					onChange={(event) => onChange(event.target.value)}
				/>
			);
	}

	return (
		<div className="flex flex-col gap-1" data-question={q.id}>
			<Label htmlFor={id}>
				{t(questionKey(set, q.id))}{" "}
				<span className="font-normal text-muted-foreground">({need})</span>
			</Label>
			{control}
			{(q.kind === "seconds" || q.kind === "words") && (
				<p id={`${id}-hint`} className="text-xs text-muted-foreground">
					{t(
						q.kind === "words"
							? "ui.projects.briefing.words_hint"
							: "ui.projects.briefing.seconds_hint",
						{ min: q.min ?? 0, max: q.max ?? 0 },
					)}
				</p>
			)}
		</div>
	);
}

// The guided briefing of a production (RF-301): the questions come from the core, the answers go back to it
// and become the brief. The page validates nothing: the service refuses a bad answer and says which.
export function Briefing({
	projectId,
	briefing,
	onSaved,
}: {
	projectId: string;
	briefing: BriefingQuestionsResult;
	onSaved(text: string): void;
}): ReactNode {
	const t = useT();
	const set = briefing.production_kind;
	const [answers, setAnswers] = useState<Record<string, string>>(
		savedAnswers(briefing),
	);
	const [replace, setReplace] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [saved, setSaved] = useState(false);
	const [busy, setBusy] = useState(false);

	const typed = JSON.stringify(answers);
	const [baseline, setBaseline] = useState(typed);
	useUnsavedGuard(typed !== baseline, t("ui.unsaved.leave"));

	const handWritten = briefing.has_brief && !briefing.guided;

	const submit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		setSaved(false);
		try {
			const client = createClient(new HttpTransport("/api/v1"));
			const res = await client.answerBriefing(projectId, answers, replace);
			setBaseline(typed);
			setSaved(true);
			onSaved(res.text);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	};

	return (
		<section aria-labelledby="briefing-title" className="flex flex-col gap-3">
			<h2 id="briefing-title" className="text-lg font-semibold">
				{t("ui.projects.briefing.title")}
			</h2>
			<p className="text-sm text-muted-foreground">{t(introKey(set))}</p>
			<form
				onSubmit={(event) => void submit(event)}
				className="flex flex-col gap-4 rounded-xl border bg-card p-4 text-card-foreground"
			>
				{briefing.questions.map((q) => (
					<Field
						key={q.id}
						q={q}
						set={set}
						value={answers[q.id] ?? ""}
						onChange={(value) => setAnswers({ ...answers, [q.id]: value })}
					/>
				))}
				{handWritten && (
					<div className="flex flex-col gap-2 text-sm">
						<p>{t("ui.projects.briefing.hand_written")}</p>
						<label className="flex items-center gap-2">
							<input
								type="checkbox"
								checked={replace}
								onChange={(event) => setReplace(event.target.checked)}
								className="accent-primary"
							/>
							{t("ui.projects.briefing.replace")}
						</label>
					</div>
				)}
				<FailureNote failure={failure} />
				{saved && (
					<p role="status" className="text-sm">
						{t("ui.projects.briefing.saved")}
					</p>
				)}
				<div>
					<Button type="submit" disabled={busy || (handWritten && !replace)}>
						{t("ui.projects.briefing.submit")}
					</Button>
				</div>
			</form>
		</section>
	);
}
