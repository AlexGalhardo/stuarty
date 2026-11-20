import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { CostSummary } from "../../editor/api/types";
import { formatMoney, parseMicros } from "./money";

function api() {
	return createClient(new HttpTransport("/api/v1"));
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
			<h2 id="approvals-cost" className="text-lg font-semibold">
				{t("ui.approvals.cost_title")}
			</h2>
			<p className="mt-1 text-sm text-muted-foreground">
				{t("ui.approvals.cost_intro")}
			</p>
			<FailureNote failure={failure} />
			{summary !== null && (
				<div className="mt-4 grid gap-4 sm:grid-cols-2">
					<div className="rounded-xl border bg-card p-4 text-card-foreground">
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
									</li>
								))}
							</ul>
						)}
					</div>
					<div className="rounded-xl border bg-card p-4 text-card-foreground">
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
			<form
				onSubmit={(event) => void save(event)}
				className="mt-4 flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
			>
				<h3 className="font-medium">{t("ui.approvals.set_ceiling")}</h3>
				<div className="grid gap-4 sm:grid-cols-4">
					<div className="flex flex-col gap-2">
						<Label htmlFor="ceiling-currency">
							{t("ui.approvals.currency")}
						</Label>
						<Input
							id="ceiling-currency"
							autoComplete="off"
							maxLength={3}
							value={currency}
							onChange={(event) => setCurrency(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="ceiling-project">
							{t("ui.approvals.rules.project")}
						</Label>
						<Input
							id="ceiling-project"
							autoComplete="off"
							placeholder={t("ui.approvals.scope_workspace")}
							value={project}
							onChange={(event) => setProject(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="ceiling-per-item">
							{t("ui.approvals.per_item")}
						</Label>
						<Input
							id="ceiling-per-item"
							inputMode="decimal"
							autoComplete="off"
							value={perItem}
							onChange={(event) => setPerItem(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="ceiling-per-day">{t("ui.approvals.per_day")}</Label>
						<Input
							id="ceiling-per-day"
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
		</section>
	);
}
