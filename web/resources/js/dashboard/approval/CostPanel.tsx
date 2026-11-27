import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { FieldLabel, HelpCard } from "@/components/FieldHelp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { CostSummary } from "../../editor/api/types";
import { formatMoney, parseMicros } from "./money";

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// The ceiling per day that applies to a line of today's spending, or null when none is set: the bar only draws the
// service's two figures against each other.
function dayCeiling(
	summary: CostSummary,
	line: { project_id?: string | null; currency: string },
): number | null {
	const found = summary.ceilings.find(
		(c) =>
			c.currency === line.currency &&
			(c.project_id ?? null) === (line.project_id ?? null),
	);
	return found?.per_day_micros ?? null;
}

// A thin bar of what was spent against the day ceiling; with no ceiling there is no bar.
function SpendBar({
	spent,
	ceiling,
	label,
}: {
	spent: number;
	ceiling: number | null;
	label: string;
}): ReactNode {
	if (ceiling === null || ceiling <= 0) return null;
	const percent = Math.min(100, Math.round((spent / ceiling) * 100));
	return (
		<span
			role="progressbar"
			aria-label={label}
			aria-valuemin={0}
			aria-valuemax={100}
			aria-valuenow={percent}
			className="mt-1 block h-2 w-full overflow-hidden rounded-full bg-surface-sunken"
		>
			<span
				className={`block h-full ${percent >= 90 ? "bg-danger-text" : "bg-accent"}`}
				style={{ width: `${percent}%` }}
			/>
		</span>
	);
}

// What was spent today and the ceilings, as the service computed them (SDD-015 section 6), with
// the form to set a ceiling. The page adds nothing: every figure is the service's.
export default function CostPanel({
	summary,
	projects = {},
	onChanged,
}: {
	summary: CostSummary | null;
	projects?: Readonly<Record<string, string>>;
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const { privileged } = useSession();
	const [failure, setFailure] = useState<Failure | null>(null);
	const [currency, setCurrency] = useState("USD");
	const [project, setProject] = useState("");
	const [perItem, setPerItem] = useState("");
	const [perDay, setPerDay] = useState("");

	async function save(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		const item = perItem.trim() === "" ? undefined : parseMicros(perItem);
		const day = perDay.trim() === "" ? undefined : parseMicros(perDay);
		if (item === null || day === null) {
			setFailure({
				code: "E_VALIDATION",
				message: t("ui.approvals.rules.bad_amount"),
			});
			return;
		}
		setFailure(null);
		try {
			await api().setCostCeiling({
				currency: currency.trim().toUpperCase(),
				...(project.trim() === "" ? {} : { project_id: project.trim() }),
				...(item === undefined ? {} : { per_item_micros: item }),
				...(day === undefined ? {} : { per_day_micros: day }),
			});
			setPerItem("");
			setPerDay("");
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	return (
		<section aria-labelledby="approvals-cost">
			<h2 id="approvals-cost" className="text-xl font-semibold">
				{t("ui.approvals.cost_title")}
			</h2>
			<p className="mt-1 text-sm text-muted-foreground">
				{t("ui.approvals.cost_intro")}
			</p>
			<FailureNote failure={failure} />
			{summary !== null && (
				<div className="mt-4 grid grid-cols-1 gap-4">
					<div className="rounded-lg border border-border p-3">
						<h3 className="font-medium">{t("ui.approvals.spent_today")}</h3>
						{summary.today.length === 0 ? (
							<p className="mt-2 text-sm text-muted-foreground">
								{t("ui.approvals.nothing_spent")}
							</p>
						) : (
							<ul className="mt-2 text-sm" data-testid="cost-today">
								{summary.today.map((line) => (
									<li
										key={`${line.scope}-${line.project_id ?? ""}-${line.currency}`}
									>
										{line.scope === "workspace"
											? t("ui.approvals.scope_workspace")
											: (projects[line.project_id ?? ""] ??
												line.project_id ??
												"")}
										{": "}
										{formatMoney(
											{ micros: line.micros, currency: line.currency },
											locale,
										)}
										<SpendBar
											spent={line.micros}
											ceiling={dayCeiling(summary, line)}
											label={t("ui.approvals.spent_bar")}
										/>
									</li>
								))}
							</ul>
						)}
					</div>
					<div className="rounded-lg border border-border p-3">
						<h3 className="font-medium">{t("ui.approvals.ceilings")}</h3>
						{summary.ceilings.length === 0 ? (
							<p className="mt-2 text-sm text-muted-foreground">
								{t("ui.approvals.no_ceilings")}
							</p>
						) : (
							<ul className="mt-2 text-sm" data-testid="cost-ceilings">
								{summary.ceilings.map((c) => (
									<li key={`${c.project_id ?? ""}-${c.currency}`}>
										{c.project_id === null
											? t("ui.approvals.scope_workspace")
											: (projects[c.project_id] ?? c.project_id)}
										{": "}
										{t("ui.approvals.rules.limits", {
											item:
												c.per_item_micros === null
													? "-"
													: formatMoney(
															{
																micros: c.per_item_micros,
																currency: c.currency,
															},
															locale,
														),
											day:
												c.per_day_micros === null
													? "-"
													: formatMoney(
															{
																micros: c.per_day_micros,
																currency: c.currency,
															},
															locale,
														),
										})}
									</li>
								))}
							</ul>
						)}
					</div>
				</div>
			)}
			<details
				className="mt-4 border-t border-border pt-4"
				data-testid="ceiling-details"
			>
				<summary className="w-fit cursor-pointer text-base font-medium">
					{t("ui.approvals.set_ceiling")}
				</summary>
				<form
					onSubmit={(event) => void save(event)}
					className="mt-4 flex flex-col gap-4"
				>
					<HelpCard
						title={t("ui.help.g3.cost_card_title")}
						description={t("ui.help.g3.cost_card_body")}
					/>
					<div className="grid grid-cols-1 gap-4">
						<div className="flex flex-col gap-2">
							<FieldLabel
								htmlFor="ceiling-currency"
								label={t("ui.approvals.currency")}
								help={t("ui.help.g3.currency_help")}
							/>
							<Input
								id="ceiling-currency"
								placeholder={t("ui.help.g3.currency_ph")}
								autoComplete="off"
								maxLength={3}
								value={currency}
								onChange={(event) => setCurrency(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<FieldLabel
								htmlFor="ceiling-project"
								label={t("ui.approvals.rules.project")}
								help={t("ui.help.g3.ceiling_project_help")}
							/>
							<Input
								id="ceiling-project"
								autoComplete="off"
								placeholder={t("ui.approvals.scope_workspace")}
								value={project}
								onChange={(event) => setProject(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<FieldLabel
								htmlFor="ceiling-per-item"
								label={t("ui.approvals.per_item")}
								help={t("ui.help.g3.per_item_help")}
							/>
							<Input
								id="ceiling-per-item"
								placeholder={t("ui.help.g3.per_item_ph")}
								inputMode="decimal"
								autoComplete="off"
								value={perItem}
								onChange={(event) => setPerItem(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<FieldLabel
								htmlFor="ceiling-per-day"
								label={t("ui.approvals.per_day")}
								help={t("ui.help.g3.per_day_help")}
							/>
							<Input
								id="ceiling-per-day"
								placeholder={t("ui.help.g3.per_day_ph")}
								inputMode="decimal"
								autoComplete="off"
								value={perDay}
								onChange={(event) => setPerDay(event.target.value)}
							/>
						</div>
					</div>
					<p className="text-xs text-muted-foreground">
						{t("ui.approvals.ceiling_note")}
					</p>
					<PersonOnlyNote id="cost-person-only" />
					<div>
						<Button
							type="submit"
							disabled={!privileged}
							aria-describedby={privileged ? undefined : "cost-person-only"}
						>
							{t("ui.approvals.save_ceiling")}
						</Button>
					</div>
				</form>
			</details>
		</section>
	);
}
