import { type ReactNode, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { useT } from "@/lib/i18n";
import { ApiError } from "../../editor/api/errors";

// The path the service names when a person schedules on a quiet day without having confirmed (SDD-020 section 30A.1).
export const QUIET_PATH = "confirm_quiet_day";

export function isQuietRefusal(error: unknown): error is ApiError {
	return (
		error instanceof ApiError &&
		error.code === "E_INVALID_STATE" &&
		error.path === QUIET_PATH
	);
}

// The person said no to a quiet day: nothing was scheduled, and nothing is an error to show.
export class QuietDeclined extends Error {
	constructor() {
		super("the person did not confirm the quiet day");
		this.name = "QuietDeclined";
	}
}

// A day marked as quiet is a rule, and a person is asked to confirm before scheduling on it (D-146). The call is made
// first without the confirmation; when the service says the day is quiet, the person is asked, and only a yes makes
// the second call, with `confirm_quiet_day`. The page never decides what a quiet day is: it only passes the yes on.
export function useQuietConfirm(): {
	run: <T>(call: (confirm: boolean) => Promise<T>) => Promise<T>;
	dialog: ReactNode;
} {
	const t = useT();
	const [asking, setAsking] = useState<string | null>(null);
	const answer = useRef<((yes: boolean) => void) | null>(null);

	function ask(message: string): Promise<boolean> {
		setAsking(message);
		return new Promise((resolve) => {
			answer.current = resolve;
		});
	}

	function settle(yes: boolean): void {
		setAsking(null);
		answer.current?.(yes);
		answer.current = null;
	}

	async function run<T>(call: (confirm: boolean) => Promise<T>): Promise<T> {
		try {
			return await call(false);
		} catch (error) {
			if (!isQuietRefusal(error)) throw error;
			if (!(await ask(error.message))) throw new QuietDeclined();
			return call(true);
		}
	}

	const dialog = (
		<Dialog
			open={asking !== null}
			onOpenChange={(open) => !open && settle(false)}
		>
			<DialogContent data-testid="quiet-confirm">
				<DialogHeader>
					<DialogTitle>{t("ui.publish.quiet.confirm_title")}</DialogTitle>
					<DialogDescription>
						{asking} {t("ui.publish.quiet.confirm_body")}
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						data-testid="quiet-confirm-no"
						onClick={() => settle(false)}
					>
						{t("ui.publish.quiet.confirm_no")}
					</Button>
					<Button
						type="button"
						data-testid="quiet-confirm-yes"
						onClick={() => settle(true)}
					>
						{t("ui.publish.quiet.confirm_yes")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);

	return { run, dialog };
}
