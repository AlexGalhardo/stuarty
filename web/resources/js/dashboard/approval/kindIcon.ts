import { type LucideIcon, Send, ShieldCheck, Sparkles } from "lucide-react";

// The icon that tells the kind of a request at a glance. It is chosen from the name of the job or the tool the service
// reported, and only for the look: a name it does not know keeps the shield of the approval queue.
export function kindIcon(kind: string): LucideIcon {
	const name = kind.toLowerCase();
	if (name.includes("publish")) return Send;
	if (name.includes("generate")) return Sparkles;
	return ShieldCheck;
}
