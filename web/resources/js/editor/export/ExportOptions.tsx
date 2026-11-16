import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { ExportCapabilities, ExportOptions } from "../api/types";

export interface Unsupported {
	code: string;
	message: string;
	hint?: string;
}

const SELECT =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50 dark:bg-input/30";

// The format names the server lists; one it adds later still shows, under its
// own name, until the catalogue has a label for it.
const FORMAT_KEYS = ["mp4", "gif", "wav", "aac"] as const;

// Turns the range on, as the in and out points of the timeline (a point that
// is not set leaves that end open), or off.
export function rangeOptions(
	value: ExportOptions,
	range: { in: number | null; out: number | null },
	on: boolean,
): ExportOptions {
	const { start: _start, end: _end, ...rest } = value;
	if (!on) return rest;
	return {
		...rest,
		...(range.in === null ? {} : { start: `${range.in}f` }),
		...(range.out === null ? {} : { end: `${range.out}f` }),
	};
}

// The export options of SDD-012 section 14: the format. Frame rate and size are
// project settings (SDD-004 section 8.1), not job arguments. Only what the
// server schema exposes is offered (`capabilities`), and a choice the
// core refuses for this machine is not predicted here: the dialog asks the core
// with a dry run and shows its own message and hint in `unsupported`.
export default function ExportOptionsFields({
	capabilities,
	value,
	disabled,
	unsupported,
	range = { in: null, out: null },
	onChange,
}: {
	capabilities: ExportCapabilities;
	value: ExportOptions;
	disabled: boolean;
	unsupported: Unsupported | null;
	// The in and out points of the timeline (frames); the range is offered when
	// one is set and the server takes `start` and `end`.
	range?: { in: number | null; out: number | null };
	onChange(next: ExportOptions): void;
}): ReactNode {
	const t = useT();
	const { formats } = capabilities;
	const rangeSet =
		capabilities.range && (range.in !== null || range.out !== null);
	if (formats.length === 0 && !rangeSet) return null;
	const format = value.format ?? "mp4";
	return (
		<fieldset
			className="mb-3 flex flex-col gap-2"
			disabled={disabled}
			data-testid="export-options"
		>
			<legend className="mb-1 text-sm font-semibold">
				{t("ui.editor.export.options.title")}
			</legend>
			{formats.length > 0 && (
				<div className="flex flex-col gap-1">
					<Label htmlFor="export-format" className="text-xs">
						{t("ui.editor.export.options.format")}
					</Label>
					<select
						id="export-format"
						className={SELECT}
						value={format}
						onChange={(event) => {
							const next = event.target.value;
							onChange({ ...value, format: next === "mp4" ? undefined : next });
						}}
					>
						{(formats.includes("mp4") ? formats : ["mp4", ...formats]).map(
							(name) => (
								<option key={name} value={name}>
									{(FORMAT_KEYS as readonly string[]).includes(name)
										? t(`ui.editor.export.options.formats.${name}`)
										: name}
								</option>
							),
						)}
					</select>
				</div>
			)}
			{rangeSet && (
				<div className="flex items-center gap-2">
					<input
						id="export-range"
						type="checkbox"
						className="size-4 accent-primary"
						checked={value.start !== undefined || value.end !== undefined}
						onChange={(event) =>
							onChange(rangeOptions(value, range, event.target.checked))
						}
					/>
					<Label htmlFor="export-range" className="text-xs">
						{t("ui.editor.export.options.range")}
					</Label>
				</div>
			)}
			{unsupported !== null && (
				<div role="alert" className="text-xs text-destructive">
					<p className="font-mono">{unsupported.code}</p>
					<p lang="en">
						<UntrustedText>{unsupported.message}</UntrustedText>
					</p>
					{unsupported.hint !== undefined && (
						<p lang="en">
							<UntrustedText>{unsupported.hint}</UntrustedText>
						</p>
					)}
				</div>
			)}
		</fieldset>
	);
}
