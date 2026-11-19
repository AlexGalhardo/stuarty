import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { parseLength } from "./units";

// The fields of the inspector (SDD-012 section 24.6). A field shows the value of the read model and sends its
// command on Enter or when it loses the focus, never on every key; Escape puts the shown value back. When the
// server refuses, the read model does not change, so the field falls back to it and the error bar says why.

// What a field sends: nothing to wait for, or a promise that resolves to null when the server refused.
export type Commit<T> = (value: T) => undefined | Promise<unknown>;

// The text a field shows while it is being edited. It follows the value of the read model whenever that changes
// (another actor moved the layer, the server answered), and falls back to it when a command is refused.
function useDraft<T>(value: T): {
	draft: T;
	edit(next: T): void;
	revert(): void;
	run<V>(commit: Commit<V>, next: V): void;
} {
	const [draft, setDraft] = useState<T>(value);
	// What was sent and not answered yet: Enter and the blur that follows it must not send it twice.
	const sent = useRef<unknown>(undefined);
	useEffect(() => {
		sent.current = undefined;
		setDraft(value);
	}, [value]);
	return {
		draft,
		edit: setDraft,
		revert: () => setDraft(value),
		run: (commit, next) => {
			if (sent.current === next) return;
			sent.current = next;
			const pending = commit(next);
			if (pending !== undefined) {
				void pending.then((result) => {
					if (result === null) {
						sent.current = undefined;
						setDraft(value);
					}
				});
			}
		},
	};
}

const labelClass = "text-xs font-normal text-muted-foreground";

export function NumberField({
	label,
	value,
	axis = 0,
	unit,
	onCommit,
	disabled,
	testId,
}: {
	label: string;
	value: number;
	// The size of the canvas along the axis, for a percent; 0 for a field where a percent has no meaning.
	axis?: number;
	unit?: string;
	onCommit: Commit<number>;
	disabled?: boolean;
	testId?: string;
}): ReactNode {
	const id = useId();
	const t = useT();
	const { draft, edit: setDraft, revert: back, run } = useDraft(String(value));
	const [bad, setBad] = useState(false);

	const commit = (): void => {
		const parsed = parseLength(draft, axis);
		if (parsed === null) {
			setBad(true);
			return;
		}
		setBad(false);
		setDraft(String(parsed.value));
		if (parsed.value !== value) run(onCommit, parsed.value);
	};
	const revert = (): void => {
		setBad(false);
		back();
	};

	return (
		<div className="flex min-w-0 flex-col gap-1">
			<Label htmlFor={id} className={labelClass}>
				{label}
			</Label>
			<div className="relative">
				<Input
					id={id}
					data-testid={testId}
					inputMode="numeric"
					value={draft}
					disabled={disabled}
					aria-invalid={bad || undefined}
					aria-describedby={bad ? `${id}-bad` : undefined}
					onChange={(event) => setDraft(event.target.value)}
					onBlur={commit}
					onKeyDown={(event) => {
						if (event.key === "Enter") {
							event.preventDefault();
							commit();
						} else if (event.key === "Escape") {
							event.preventDefault();
							revert();
						}
					}}
					className={cn("tabular-nums", unit !== undefined && "pr-8")}
				/>
				{unit !== undefined && (
					<span
						aria-hidden="true"
						className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-xs text-foreground"
					>
						{unit}
					</span>
				)}
			</div>
			{bad && (
				<p id={`${id}-bad`} role="alert" className="text-xs text-destructive">
					{t("ui.image.inspector.bad_number")}
				</p>
			)}
		</div>
	);
}

export function TextField({
	label,
	value,
	onCommit,
	disabled,
	multiline,
	maxLength,
	testId,
}: {
	label: string;
	value: string;
	onCommit: Commit<string>;
	disabled?: boolean;
	multiline?: boolean;
	maxLength?: number;
	testId?: string;
}): ReactNode {
	const id = useId();
	const { draft, edit: setDraft, run } = useDraft(value);
	const commit = (): void => {
		if (draft !== value) run(onCommit, draft);
	};
	const common = {
		id,
		"data-testid": testId,
		value: draft,
		disabled,
		maxLength,
		onChange: (event: { target: { value: string } }) =>
			setDraft(event.target.value),
		onBlur: commit,
	};

	return (
		<div className="flex min-w-0 flex-col gap-1">
			<Label htmlFor={id} className={labelClass}>
				{label}
			</Label>
			{multiline ? (
				<textarea
					{...common}
					rows={3}
					onKeyDown={(event) => {
						if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
							event.preventDefault();
							commit();
						}
					}}
					className="min-h-16 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
				/>
			) : (
				<Input
					{...common}
					onKeyDown={(event) => {
						if (event.key === "Enter") {
							event.preventDefault();
							commit();
						}
					}}
				/>
			)}
		</div>
	);
}

export function SelectField({
	label,
	value,
	options,
	onCommit,
	disabled,
	testId,
}: {
	label: string;
	value: string;
	options: readonly { value: string; label: string }[];
	onCommit: Commit<string>;
	disabled?: boolean;
	testId?: string;
}): ReactNode {
	const id = useId();
	return (
		<div className="flex min-w-0 flex-col gap-1">
			<Label htmlFor={id} className={labelClass}>
				{label}
			</Label>
			<select
				id={id}
				data-testid={testId}
				value={value}
				disabled={disabled}
				onChange={(event) => void onCommit(event.target.value)}
				className="h-8 w-full min-w-0 rounded-lg border border-input bg-background px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
			>
				{options.map((option) => (
					<option key={option.value} value={option.value}>
						{option.label}
					</option>
				))}
			</select>
		</div>
	);
}

export function ToggleField({
	label,
	checked,
	onCommit,
	disabled,
	testId,
}: {
	label: string;
	checked: boolean;
	onCommit: Commit<boolean>;
	disabled?: boolean;
	testId?: string;
}): ReactNode {
	const id = useId();
	return (
		<div className="flex items-center gap-2">
			<input
				id={id}
				type="checkbox"
				data-testid={testId}
				checked={checked}
				disabled={disabled}
				onChange={(event) => void onCommit(event.target.checked)}
				className="size-4 accent-[var(--primary)]"
			/>
			<Label htmlFor={id} className="text-sm font-normal">
				{label}
			</Label>
		</div>
	);
}

// A colour: the text of the colour, a swatch to pick one, and the swatches of the brand kit when it has any.
// The text is what is sent; the core checks that it is a colour.
export function ColorField({
	label,
	value,
	swatches,
	onCommit,
	allowEmpty,
	disabled,
	testId,
}: {
	label: string;
	value: string;
	swatches?: readonly { name: string; value: string }[];
	onCommit: Commit<string>;
	allowEmpty?: boolean;
	disabled?: boolean;
	testId?: string;
}): ReactNode {
	const id = useId();
	const t = useT();
	const { draft, edit: setDraft, run } = useDraft(value);
	const commit = (): void => {
		const text = draft.trim();
		if (text === value || (text === "" && !allowEmpty)) return;
		run(onCommit, text);
	};
	const picked = /^#[0-9a-fA-F]{6}/.test(value) ? value.slice(0, 7) : "#000000";

	return (
		<div className="flex min-w-0 flex-col gap-1">
			<Label htmlFor={id} className={labelClass}>
				{label}
			</Label>
			<div className="flex items-center gap-1">
				<input
					type="color"
					aria-label={t("ui.image.inspector.pick_color", { label })}
					value={picked}
					disabled={disabled}
					onChange={(event) => void onCommit(event.target.value.toUpperCase())}
					className="size-8 shrink-0 cursor-pointer rounded border bg-transparent p-0.5"
				/>
				<Input
					id={id}
					data-testid={testId}
					value={draft}
					disabled={disabled}
					placeholder={allowEmpty ? t("ui.image.inspector.none") : undefined}
					onChange={(event) => setDraft(event.target.value)}
					onBlur={commit}
					onKeyDown={(event) => {
						if (event.key === "Enter") {
							event.preventDefault();
							commit();
						}
					}}
					className="font-mono text-sm"
				/>
			</div>
			{swatches !== undefined && swatches.length > 0 && (
				<fieldset className="m-0 flex min-w-0 flex-wrap gap-1 border-0 p-0">
					<legend className="sr-only">
						{t("ui.image.brand.swatches", { label })}
					</legend>
					{swatches.map((swatch) => (
						<button
							key={swatch.name}
							type="button"
							title={`${swatch.name} ${swatch.value}`}
							aria-label={t("ui.image.brand.use_color", {
								name: swatch.name,
								value: swatch.value,
							})}
							disabled={disabled}
							data-testid="image-brand-swatch"
							onClick={() => void onCommit(swatch.value)}
							className="size-6 rounded border outline-none focus-visible:ring-2 focus-visible:ring-ring"
							style={{ background: swatch.value }}
						/>
					))}
				</fieldset>
			)}
		</div>
	);
}

export function Section({
	title,
	children,
	testId,
}: {
	title: string;
	children: ReactNode;
	testId?: string;
}): ReactNode {
	const id = useId();
	return (
		<section
			aria-labelledby={id}
			data-testid={testId}
			className="flex flex-col gap-2 border-b p-3"
		>
			<h3 id={id} className="text-xs font-semibold uppercase tracking-wide">
				{title}
			</h3>
			{children}
		</section>
	);
}
