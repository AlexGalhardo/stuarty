import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale, useT } from "@/lib/i18n";
import { repeatOperations } from "../commands/builders";
import { registryLimits } from "../commands/registryLimits";
import { localizeDecimalInput, normalizeTimeInput } from "../commands/units";
import { useActions } from "../commands/useActions";
import { useRuntime, useUiState } from "../state/EditorContext";
import { readTimeUnit } from "../state/prefs";

const SELECT_CLASS =
	"h-8 min-w-0 rounded-lg border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30";

type Mode = "copies" | "until";

// The Repeat dialog of the item menu (`item.repeat`): how many extra copies, or
// the time the sequence ends at, whether the later items move, and whether the
// copies are linked. One command per confirmation; the core judges the rest and
// its refusal is shown here.
function RepeatForm({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const locale = useLocale();
	const runtime = useRuntime();
	const actions = useActions();
	const limits = registryLimits("item.repeat", "copies");
	const [mode, setMode] = useState<Mode>("copies");
	const [copies, setCopies] = useState("1");
	const [until, setUntil] = useState("");
	const [ripple, setRipple] = useState(false);
	const [link, setLink] = useState(false);
	const [error, setError] = useState<string | null>(null);

	const close = (): void => runtime.uiWriter.startRepeat(null);
	const submit = async (): Promise<void> => {
		const common = {
			ripple: ripple ? ("track" as const) : ("none" as const),
			link,
		};
		if (mode === "copies") {
			const count = Number(copies);
			if (
				!Number.isInteger(count) ||
				count < (limits.min ?? 1) ||
				count > (limits.max ?? 200)
			) {
				setError(t("ui.editor.repeat.invalid_copies"));
				return;
			}
			return finish({ copies: count, ...common });
		}
		const time = normalizeTimeInput(
			localizeDecimalInput(until, locale),
			readTimeUnit(),
		);
		if (!time.ok) {
			setError(t("ui.editor.repeat.invalid_until"));
			return;
		}
		return finish({ until: time.value, ...common });
	};
	const finish = async (
		input: Parameters<typeof repeatOperations>[1],
	): Promise<void> => {
		const outcome = await actions.commitField(repeatOperations(itemId, input));
		if (outcome === null || outcome.ok) {
			close();
			return;
		}
		setError(outcome.error.message);
	};

	return (
		<form
			className="flex flex-col gap-3"
			onSubmit={(event) => {
				event.preventDefault();
				void submit();
			}}
		>
			<div className="grid grid-cols-[96px_1fr] items-center gap-x-2 gap-y-2">
				<Label htmlFor="repeat-mode" className="text-xs">
					{t("ui.editor.repeat.mode")}
				</Label>
				<select
					id="repeat-mode"
					value={mode}
					className={SELECT_CLASS}
					onChange={(event) =>
						setMode(event.target.value === "until" ? "until" : "copies")
					}
				>
					<option value="copies">{t("ui.editor.repeat.mode_copies")}</option>
					<option value="until">{t("ui.editor.repeat.mode_until")}</option>
				</select>
				{mode === "copies" ? (
					<>
						<Label htmlFor="repeat-copies" className="text-xs">
							{t("ui.editor.repeat.copies")}
						</Label>
						<Input
							id="repeat-copies"
							value={copies}
							inputMode="numeric"
							autoComplete="off"
							className="h-8"
							onChange={(event) => setCopies(event.target.value)}
						/>
					</>
				) : (
					<>
						<Label htmlFor="repeat-until" className="text-xs">
							{t("ui.editor.repeat.until")}
						</Label>
						<Input
							id="repeat-until"
							value={until}
							autoComplete="off"
							className="h-8"
							onChange={(event) => setUntil(event.target.value)}
						/>
					</>
				)}
				<Label htmlFor="repeat-ripple" className="text-xs">
					{t("ui.editor.repeat.ripple")}
				</Label>
				<input
					id="repeat-ripple"
					type="checkbox"
					checked={ripple}
					className="size-4 accent-primary"
					onChange={(event) => setRipple(event.target.checked)}
				/>
				<Label htmlFor="repeat-link" className="text-xs">
					{t("ui.editor.repeat.link")}
				</Label>
				<input
					id="repeat-link"
					type="checkbox"
					checked={link}
					className="size-4 accent-primary"
					onChange={(event) => setLink(event.target.checked)}
				/>
			</div>
			{error !== null && (
				<p role="alert" className="text-xs text-destructive">
					{error}
				</p>
			)}
			<DialogFooter>
				<Button type="button" variant="outline" onClick={close}>
					{t("ui.editor.repeat.cancel")}
				</Button>
				<Button type="submit">{t("ui.editor.repeat.confirm")}</Button>
			</DialogFooter>
		</form>
	);
}

export default function RepeatDialog(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const itemId = useUiState((s) => s.repeatingItem);
	return (
		<Dialog
			open={itemId !== null}
			onOpenChange={(open) => {
				if (!open) runtime.uiWriter.startRepeat(null);
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t("ui.editor.repeat.title")}</DialogTitle>
					<DialogDescription>{t("ui.editor.repeat.hint")}</DialogDescription>
				</DialogHeader>
				{itemId !== null && <RepeatForm key={itemId} itemId={itemId} />}
			</DialogContent>
		</Dialog>
	);
}
