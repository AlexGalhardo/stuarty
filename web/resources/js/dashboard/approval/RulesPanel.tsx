import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale, useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	ApprovalRule,
	ApprovalRuleSetRequest,
} from "../../editor/api/types";
import { formatMoney, parseMicros } from "./money";

const SELECT =
	"h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50";

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

function limit(
	micros: number | null,
	currency: string | undefined,
	locale: string,
): string | null {
	if (micros === null) return null;
	return formatMoney({ micros, currency: currency ?? "USD" }, locale);
}

// One rule of automatic release: the kind of action, where and for which project it applies, and
// the limits per item and per day, as the service stores them.
export function RuleRow({
	rule,
	destination,
	onRemove,
	onToggle,
}: {
	rule: ApprovalRule;
	destination: string | null;
	onRemove(): void;
	onToggle(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const perItem = limit(rule.per_item_micros, rule.currency, locale);
	const perDay = limit(rule.per_day_micros, rule.currency, locale);
	return (
		<li
			className="flex items-start justify-between gap-4 p-4"
			data-rule-id={rule.id}
			data-testid="rule-row"
		>
			<div className="min-w-0">
				<p className="font-medium">
					<UntrustedText>{rule.kind}</UntrustedText>
					{!rule.enabled && (
						<span className="ml-2 text-xs font-normal text-muted-foreground">
							{t("ui.approvals.rules.disabled")}
						</span>
					)}
				</p>
				<p className="text-xs text-muted-foreground">
					{t("ui.approvals.rules.applies", {
						destination:
							destination ??
							rule.connection_id ??
							t("ui.approvals.rules.any_destination"),
						project: rule.project_id ?? t("ui.approvals.rules.any_project"),
					})}
				</p>
				<p className="text-xs text-muted-foreground">
					{perItem === null && perDay === null
						? t("ui.approvals.rules.no_limits")
						: t("ui.approvals.rules.limits", {
								item: perItem ?? "-",
								day: perDay ?? "-",
							})}
				</p>
			</div>
			<div className="flex shrink-0 gap-2">
				<Button type="button" variant="outline" onClick={onToggle}>
					{rule.enabled
						? t("ui.approvals.rules.disable")
						: t("ui.approvals.rules.enable")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{rule.kind}</UntrustedText>
					</span>
				</Button>
				<Button type="button" variant="outline" onClick={onRemove}>
					{t("ui.approvals.rules.remove")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{rule.kind}</UntrustedText>
					</span>
				</Button>
			</div>
		</li>
	);
}

// The rules screen (RF-136): the rules a person wrote that approve a kind of action without
// asking. A rule is written here and nowhere else; an agent has no way to write one.
export default function RulesPanel({
	rules,
	destinations,
	onChanged,
}: {
	rules: readonly ApprovalRule[];
	destinations: Readonly<Record<string, string>>;
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const [failure, setFailure] = useState<Failure | null>(null);
	const [kind, setKind] = useState("");
	const [connection, setConnection] = useState("");
	const [project, setProject] = useState("");
	const [currency, setCurrency] = useState("USD");
	const [perItem, setPerItem] = useState("");
	const [perDay, setPerDay] = useState("");

	async function run(action: () => Promise<unknown>): Promise<void> {
		setFailure(null);
		try {
			await action();
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	function request(): ApprovalRuleSetRequest | Failure {
		const item = perItem.trim() === "" ? undefined : parseMicros(perItem);
		const day = perDay.trim() === "" ? undefined : parseMicros(perDay);
		if (item === null || day === null) {
			return {
				code: "E_VALIDATION",
				message: t("ui.approvals.rules.bad_amount"),
			};
		}
		return {
			kind: kind.trim(),
			...(connection === "" ? {} : { connection_id: connection }),
			...(project.trim() === "" ? {} : { project_id: project.trim() }),
			...(item === undefined && day === undefined
				? {}
				: { currency: currency.trim().toUpperCase() }),
			...(item === undefined ? {} : { per_item_micros: item }),
			...(day === undefined ? {} : { per_day_micros: day }),
		};
	}

	async function create(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		const body = request();
		if ("code" in body) {
			setFailure(body);
			return;
		}
		await run(async () => {
			await api().setApprovalRule(body);
			setKind("");
			setPerItem("");
			setPerDay("");
		});
	}

	return (
		<section aria-labelledby="approvals-rules">
			<h2 id="approvals-rules" className="text-lg font-semibold">
				{t("ui.approvals.rules.title")}
			</h2>
			<p className="mt-1 text-sm text-muted-foreground">
				{t("ui.approvals.rules.intro")}
			</p>
			<FailureNote failure={failure} />
			{rules.length === 0 ? (
				<p className="mt-4 text-sm text-muted-foreground">
					{t("ui.approvals.rules.empty")}
				</p>
			) : (
				<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
					{rules.map((rule) => (
						<RuleRow
							key={rule.id}
							rule={rule}
							destination={
								rule.connection_id === null
									? null
									: (destinations[rule.connection_id] ?? null)
							}
							onRemove={() => void run(() => api().removeApprovalRule(rule.id))}
							onToggle={() =>
								void run(() =>
									api().setApprovalRule({
										id: rule.id,
										kind: rule.kind,
										...(rule.connection_id === null
											? {}
											: { connection_id: rule.connection_id }),
										...(rule.project_id === null
											? {}
											: { project_id: rule.project_id }),
										...(rule.currency === undefined
											? {}
											: { currency: rule.currency }),
										...(rule.per_item_micros === null
											? {}
											: { per_item_micros: rule.per_item_micros }),
										...(rule.per_day_micros === null
											? {}
											: { per_day_micros: rule.per_day_micros }),
										enabled: !rule.enabled,
									}),
								)
							}
						/>
					))}
				</ul>
			)}
			<form
				onSubmit={(event) => void create(event)}
				className="mt-4 flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
			>
				<h3 className="font-medium">{t("ui.approvals.rules.new")}</h3>
				<div className="flex flex-col gap-2">
					<Label htmlFor="rule-kind">{t("ui.approvals.rules.kind")}</Label>
					<Input
						id="rule-kind"
						required
						autoComplete="off"
						maxLength={80}
						value={kind}
						onChange={(event) => setKind(event.target.value)}
					/>
				</div>
				<div className="flex flex-col gap-2">
					<Label htmlFor="rule-connection">
						{t("ui.approvals.rules.destination")}
					</Label>
					<select
						id="rule-connection"
						className={SELECT}
						value={connection}
						onChange={(event) => setConnection(event.target.value)}
					>
						<option value="">{t("ui.approvals.rules.any_destination")}</option>
						{Object.entries(destinations).map(([id, label]) => (
							<option key={id} value={id}>
								{label}
							</option>
						))}
					</select>
				</div>
				<div className="flex flex-col gap-2">
					<Label htmlFor="rule-project">
						{t("ui.approvals.rules.project")}
					</Label>
					<Input
						id="rule-project"
						autoComplete="off"
						placeholder={t("ui.approvals.rules.any_project")}
						value={project}
						onChange={(event) => setProject(event.target.value)}
					/>
				</div>
				<div className="grid gap-4 sm:grid-cols-3">
					<div className="flex flex-col gap-2">
						<Label htmlFor="rule-currency">{t("ui.approvals.currency")}</Label>
						<Input
							id="rule-currency"
							autoComplete="off"
							maxLength={3}
							value={currency}
							onChange={(event) => setCurrency(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="rule-per-item">{t("ui.approvals.per_item")}</Label>
						<Input
							id="rule-per-item"
							inputMode="decimal"
							autoComplete="off"
							value={perItem}
							onChange={(event) => setPerItem(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="rule-per-day">{t("ui.approvals.per_day")}</Label>
						<Input
							id="rule-per-day"
							inputMode="decimal"
							autoComplete="off"
							value={perDay}
							onChange={(event) => setPerDay(event.target.value)}
						/>
					</div>
				</div>
				<p className="text-xs text-muted-foreground">
					{t("ui.approvals.rules.note")}
				</p>
				<div>
					<Button type="submit" disabled={kind.trim() === ""}>
						{t("ui.approvals.rules.add")}
					</Button>
				</div>
			</form>
		</section>
	);
}
