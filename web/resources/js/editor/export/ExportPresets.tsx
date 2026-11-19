import { CircleAlert, CircleCheck, Clock } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type {
	CodecAvailability,
	ExportCheckResult,
	ExportPreset,
} from "../api/types";
import { groupByNetwork, limitLines } from "./presetText";

const SELECT =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50 dark:bg-input/30";

const CODECS = ["h264", "hevc", "av1"] as const;

// "pending" is a check that has not answered yet.
export type PresetCheck = ExportCheckResult | "pending";

function PresetCheckLine({
	check,
}: {
	check: PresetCheck | undefined;
}): ReactNode {
	const t = useT();
	if (check === undefined) return null;
	if (check === "pending") {
		return (
			<p className="flex items-center gap-1 text-xs text-muted-foreground">
				<Clock aria-hidden="true" className="size-3.5" />
				{t("ui.editor.export.presets.checking")}
			</p>
		);
	}
	if (check.ok) {
		return (
			<p className="flex items-center gap-1 text-xs text-success">
				<CircleCheck aria-hidden="true" className="size-3.5" />
				{t("ui.editor.export.presets.fits")}
			</p>
		);
	}
	return (
		<div role="alert" className="text-xs text-destructive">
			<p className="flex items-center gap-1 font-medium">
				<CircleAlert aria-hidden="true" className="size-3.5" />
				{t("ui.editor.export.presets.does_not_fit")}
			</p>
			<ul className="list-disc pl-5">
				{check.findings.map((finding) => (
					<li key={finding.code}>
						<span className="font-mono">{finding.code}</span>{" "}
						<span lang="en">
							<UntrustedText>{finding.message}</UntrustedText>
						</span>{" "}
						<span className="text-muted-foreground">
							{t("ui.editor.export.presets.limit_line", {
								limit: finding.limit,
								got: finding.got,
							})}
						</span>
					</li>
				))}
			</ul>
		</div>
	);
}

// The platform presets of the dialog (RF-087, RF-088): grouped by network with
// their limits and the date those limits were read, several at once, each with
// what the core says about the project against it (ExportCheck), and the video
// codec with what this machine can write. The browser decides nothing: the
// limits, the findings and the codecs are the core's.
export default function ExportPresets({
	presets,
	selected,
	checks,
	codecs,
	codec,
	codecDisabled,
	disabled,
	onToggle,
	onCodec,
}: {
	presets: readonly ExportPreset[];
	selected: readonly string[];
	checks: Readonly<Record<string, PresetCheck>>;
	// What this machine writes, when the core has said.
	codecs: readonly CodecAvailability[] | null;
	codec: string;
	// The codec is for an mp4: a plain export of another format has none to choose.
	codecDisabled: boolean;
	disabled: boolean;
	onToggle(id: string): void;
	onCodec(codec: string): void;
}): ReactNode {
	const t = useT();
	const misfits = selected.filter((id) => {
		const check = checks[id];
		return check !== undefined && check !== "pending" && !check.ok;
	}).length;
	return (
		<fieldset
			className="mb-3 flex flex-col gap-2"
			disabled={disabled}
			data-testid="export-presets"
		>
			<legend className="mb-1 text-sm font-semibold">
				{t("ui.editor.export.presets.title")}
			</legend>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.export.presets.hint")}
			</p>
			{groupByNetwork(presets).map((group) => (
				<fieldset key={group.network} className="flex flex-col gap-1.5">
					<legend className="text-xs font-semibold">{group.network}</legend>
					{group.presets.map((preset) => {
						const checked = selected.includes(preset.id);
						const source = preset.sources[0];
						return (
							<div key={preset.id} className="flex flex-col gap-0.5">
								<div className="flex items-center gap-2">
									<input
										id={`export-preset-${preset.id}`}
										type="checkbox"
										className="size-4 accent-primary"
										checked={checked}
										onChange={() => onToggle(preset.id)}
									/>
									<Label
										htmlFor={`export-preset-${preset.id}`}
										className="text-sm"
									>
										{preset.label}
									</Label>
								</div>
								<p className="pl-6 text-xs text-muted-foreground">
									{limitLines(t, preset).join(" · ")}
									{source !== undefined && (
										<>
											{" · "}
											<a
												href={source.url}
												target="_blank"
												rel="noopener noreferrer"
												className="underline"
											>
												{t("ui.editor.export.presets.source_link")}
											</a>{" "}
											{t("ui.editor.export.presets.source", {
												date: source.read,
											})}
										</>
									)}
								</p>
								{checked && (
									<div className="pl-6">
										<PresetCheckLine check={checks[preset.id]} />
									</div>
								)}
							</div>
						);
					})}
				</fieldset>
			))}
			{misfits > 0 && (
				<p role="status" className="text-xs text-warning">
					{t("ui.editor.export.presets.misfit", { count: misfits })}
				</p>
			)}
			<div className="flex flex-col gap-1">
				<Label htmlFor="export-codec" className="text-xs">
					{t("ui.editor.export.codec.label")}
				</Label>
				<select
					id="export-codec"
					className={SELECT}
					value={codec}
					disabled={codecDisabled}
					onChange={(event) => onCodec(event.target.value)}
				>
					<option value="">{t("ui.editor.export.codec.auto")}</option>
					{CODECS.map((name) => {
						const info = codecs?.find((c) => c.codec === name);
						const missing = info !== undefined && !info.available;
						const label = t(`ui.editor.export.codec.${name}`);
						return (
							<option key={name} value={name} disabled={missing}>
								{missing
									? t("ui.editor.export.codec.unavailable", { name: label })
									: label}
							</option>
						);
					})}
				</select>
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.export.codec.fallback_note")}
				</p>
			</div>
		</fieldset>
	);
}
