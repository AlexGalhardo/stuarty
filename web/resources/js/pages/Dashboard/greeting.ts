import type { LocalAccount } from "@/types";

export type DayPart = "morning" | "afternoon" | "evening";

// The part of the day of a local hour, 0 to 23. It only chooses a word for the greeting: nothing is decided from it.
export function dayPart(hour: number): DayPart {
	if (hour >= 5 && hour < 12) return "morning";
	if (hour >= 12 && hour < 18) return "afternoon";
	return "evening";
}

// The name the home greets: the username of the local account, which the server already shares with every page,
// with its first letter in capitals. Network mode and a locked or missing account have no name to greet, and the
// greeting then says only the part of the day.
export function greetingName(account: LocalAccount | undefined): string | null {
	if (account === undefined || account.state !== "unlocked") return null;
	const name = account.username.trim();
	if (name === "") return null;
	const first = name.charAt(0);
	return first.toLocaleUpperCase() + name.slice(1);
}
