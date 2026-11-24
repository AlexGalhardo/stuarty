import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";

// The recovery key as text a person can select, with the button that copies it. The key is held in the memory
// of the page only: it is in no address, history or storage (SDD-012 section 2.1.1).
export default function RecoveryKeyBox({
	value,
}: {
	value: string;
}): ReactNode {
	const t = useT();
	const [copied, setCopied] = useState(false);

	async function copy(): Promise<void> {
		try {
			await navigator.clipboard.writeText(value);
			setCopied(true);
		} catch {
			setCopied(false);
		}
	}

	return (
		<>
			<p
				data-testid="recovery-key"
				className="rounded-lg border bg-muted p-4 text-center font-mono text-lg tracking-wide break-all select-all"
			>
				{value}
			</p>
			<div className="flex items-center gap-3">
				<Button type="button" variant="outline" onClick={() => void copy()}>
					{t("ui.account.recovery.copy")}
				</Button>
				<span role="status" className="text-sm text-muted-foreground">
					{copied ? t("ui.account.recovery.copied") : ""}
				</span>
			</div>
		</>
	);
}
