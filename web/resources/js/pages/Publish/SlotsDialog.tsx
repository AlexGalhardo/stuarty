import { type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { FieldLabel, HelpCard } from "@/components/FieldHelp";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { SocialAccount, SocialNetwork } from "../../editor/api/types";
import type { CalendarApi } from "./calendarApi";
import { type SlotRule, WEEKDAYS } from "./dayTypes";

const SELECT =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2 text-sm";

// The weekly slots (SDD-020 section 30A.2): "every Tuesday 18:00 on Instagram". A slot is a reminder that appears as an
// empty slot in the calendar; it holds no post and sends nothing. The page sends what the person typed and lists what
// the service keeps; it checks no time or zone itself.
export function SlotsDialog({
	rules,
	networks,
	accounts,
	zone,
	calendarApi,
	onChanged,
	onClose,
}: {
	rules: readonly SlotRule[];
	networks: readonly SocialNetwork[];
	accounts: readonly SocialAccount[];
	zone: string;
	calendarApi: CalendarApi;
	onChanged(): void;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const [network, setNetwork] = useState(networks[0]?.network ?? "");
	const [weekday, setWeekday] = useState<string>("tue");
	const [time, setTime] = useState("18:00");
	const [where, setWhere] = useState(zone);
	const [account, setAccount] = useState("");
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const named = (id: string): string =>
		networks.find((n) => n.network === id)?.name ?? id;

	async function run(action: () => Promise<unknown>): Promise<void> {
		setBusy(true);
		setFailure(null);
		try {
			await action();
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent
				className="max-h-[90vh] overflow-y-auto sm:max-w-xl"
				data-testid="slots-dialog"
			>
				<DialogHeader>
					<DialogTitle>{t("ui.publish.slots.title")}</DialogTitle>
					<DialogDescription>{t("ui.publish.slots.intro")}</DialogDescription>
				</DialogHeader>
				<HelpCard
					title={t("ui.help.g2.slots_card_title")}
					description={t("ui.help.g2.slots_card_body")}
				/>
				{rules.length === 0 ? (
					<p
						className="text-sm text-muted-foreground"
						data-testid="slots-empty"
					>
						{t("ui.publish.slots.none")}
					</p>
				) : (
					<ul className="flex flex-col gap-2" data-testid="slots-list">
						{rules.map((r) => (
							<li
								key={r.id}
								className="flex items-center justify-between gap-2 rounded-md border px-3 py-2 text-sm"
								data-testid="slot-rule"
								data-slot-id={r.id}
							>
								<span>
									{t("ui.publish.slots.rule", {
										network: named(r.network),
										weekday: t(`ui.publish.slots.weekday_${r.weekday}`),
										time: r.time,
										zone: r.zone,
									})}
								</span>
								<Button
									type="button"
									variant="outline"
									size="sm"
									disabled={busy}
									data-testid="slot-remove"
									onClick={() => void run(() => calendarApi.removeSlot(r.id))}
								>
									{t("ui.publish.slots.remove")}
								</Button>
							</li>
						))}
					</ul>
				)}
				<form
					className="grid gap-3 sm:grid-cols-2"
					onSubmit={(event) => {
						event.preventDefault();
						void run(() =>
							calendarApi.saveSlot({
								network,
								weekday,
								time,
								zone: where,
								...(account === "" ? {} : { connection_id: account }),
							}),
						);
					}}
				>
					<div className="flex flex-col gap-1">
						<Label htmlFor="slot-network">
							{t("ui.publish.slots.network")}
						</Label>
						<select
							id="slot-network"
							className={SELECT}
							value={network}
							onChange={(event) => {
								setNetwork(event.target.value);
								setAccount("");
							}}
						>
							{networks.map((n) => (
								<option key={n.network} value={n.network}>
									{n.name}
								</option>
							))}
						</select>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="slot-weekday">
							{t("ui.publish.slots.weekday")}
						</Label>
						<select
							id="slot-weekday"
							className={SELECT}
							value={weekday}
							onChange={(event) => setWeekday(event.target.value)}
						>
							{WEEKDAYS.map((w) => (
								<option key={w} value={w}>
									{t(`ui.publish.slots.weekday_${w}`)}
								</option>
							))}
						</select>
					</div>
					<div className="flex flex-col gap-1">
						<FieldLabel
							htmlFor="slot-time"
							label={t("ui.publish.slots.time")}
							help={t("ui.help.g2.slots_time_help")}
						/>
						<Input
							id="slot-time"
							type="time"
							required
							value={time}
							onChange={(event) => setTime(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-1">
						<FieldLabel
							htmlFor="slot-zone"
							label={t("ui.publish.editor.zone")}
							help={t("ui.help.g2.slots_zone_help")}
						/>
						<Input
							id="slot-zone"
							placeholder={t("ui.help.g2.slots_zone_ph")}
							autoComplete="off"
							value={where}
							onChange={(event) => setWhere(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-1 sm:col-span-2">
						<Label htmlFor="slot-account">
							{t("ui.publish.slots.account")}
						</Label>
						<select
							id="slot-account"
							className={SELECT}
							value={account}
							onChange={(event) => setAccount(event.target.value)}
						>
							<option value="">{t("ui.publish.slots.any_account")}</option>
							{accounts
								.filter((a) => a.network === network)
								.map((a) => (
									<option key={a.connection_id} value={a.connection_id}>
										{a.label}
									</option>
								))}
						</select>
					</div>
					<FailureNote failure={failure} />
					<div className="flex gap-2 sm:col-span-2">
						<Button
							type="submit"
							disabled={busy || network === ""}
							data-testid="slot-add"
						>
							{t("ui.publish.slots.add")}
						</Button>
						<Button type="button" variant="outline" onClick={onClose}>
							{t("ui.social.close")}
						</Button>
					</div>
				</form>
			</DialogContent>
		</Dialog>
	);
}
