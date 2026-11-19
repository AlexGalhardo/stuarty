import { Command } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { isRowKey } from "../keymapStore";
import Palette from "./Palette";

// The top-bar entry to the command palette (SDD-012 section 6.2): the button
// and Mod+K, which works inside editable fields too (section 18.2).
export default function PaletteButton(): ReactNode {
	const t = useT();
	const [open, setOpen] = useState(false);

	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (!isRowKey(event, "palette")) return;
			event.preventDefault();
			setOpen((was) => !was);
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, []);

	return (
		<>
			<Button
				type="button"
				size="sm"
				variant="outline"
				aria-label={t("ui.editor.palette.open")}
				title={t("ui.editor.palette.open_hint")}
				onClick={() => setOpen(true)}
			>
				<Command aria-hidden="true" />
				<span className="hidden min-[1440px]:inline">
					{t("ui.editor.palette.label")}
				</span>
			</Button>
			<Palette open={open} onOpenChange={setOpen} />
		</>
	);
}
