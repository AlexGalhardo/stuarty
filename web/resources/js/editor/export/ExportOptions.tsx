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

// The export options of SDD-012 section 14: format, frame rate and size. Only
// what the server schema exposes is offered (`capabilities`), and a choice the
// core refuses for this machine is not predicted here: the dialog asks the core
// with a dry run and shows its own message and hint in `unsupported`.
export default function ExportOptionsFields({
	capabilities,
	value,
	disabled,
	unsupported,
	onChange,
}: {
	capabilities: ExportCapabilities;
	value: ExportOptions;
	disabled: boolean;
	unsupported: Unsupported | null;
	onChange(next: ExportOptions): void;
}): ReactNode {
	const t = useT();
	const { formats, fps, sizes } = capabilities;
	if (formats.length === 0 && fps.length === 0 && sizes.length === 0) {
		return null;
	}
	const format = value.format ?? "mp4";
	const audioOnly = format === "wav" || format === "aac";
	return (
		<fieldset
			className="mb-3 grid grid-cols-3 gap-2"
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
							// Audio has no frames: the rate and the size do not apply.
							onChange(
								next === "wav" || next === "aac"
									? { format: next }
									: {
											...value,
											format: next === "mp4" ? undefined : next,
										},
							);
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
			{fps.length > 0 && (
				<div className="flex flex-col gap-1">
					<Label htmlFor="export-fps" className="text-xs">
						{t("ui.editor.export.options.fps")}
					</Label>
					<select
						id="export-fps"
						className={SELECT}
						value={value.fps === undefined ? "" : String(value.fps)}
						disabled={audioOnly}
						onChange={(event) =>
							onChange({
								...value,
								fps:
									event.target.value === ""
										? undefined
										: Number(event.target.value),
							})
						}
					>
						<option value="">
							{t("ui.editor.export.options.project_rate")}
						</option>
						{fps.map((rate) => (
							<option key={rate} value={String(rate)}>
								{t("ui.editor.export.options.fps_value", { fps: rate })}
							</option>
						))}
					</select>
				</div>
			)}
			{sizes.length > 0 && (
				<div className="flex flex-col gap-1">
					<Label htmlFor="export-size" className="text-xs">
						{t("ui.editor.export.options.size")}
					</Label>
					<select
						id="export-size"
						className={SELECT}
						value={value.size ?? ""}
						disabled={audioOnly}
						onChange={(event) =>
							onChange({
								...value,
								size:
									event.target.value === "" ? undefined : event.target.value,
							})
						}
					>
						<option value="">
							{t("ui.editor.export.options.project_size")}
						</option>
						{sizes.map((size) => (
							<option key={size} value={size}>
								{size}
							</option>
						))}
					</select>
				</div>
			)}
			{unsupported !== null && (
				<div role="alert" className="col-span-3 text-xs text-destructive">
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
