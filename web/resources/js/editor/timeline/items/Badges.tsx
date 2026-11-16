import { EyeOff, Link2, Puzzle, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { speedLabel } from "../../commands/units";
import { useDocumentState } from "../../state/EditorContext";

// The reserved asset of an unfilled media slot (SDD-001 section 3.2).
const PLACEHOLDER_ASSET = "ast_placeholder";

// What sits on an item beside its label (SDD-012 section 7.4): the speed
// when it is not 1, the slot icon, and a warning when the asset is not in the
// document. Transition marks and keyframe diamonds are drawn by their own
// components. Everything is read from the read model; nothing is computed.
export default function Badges({ id }: { id: string }): ReactNode {
	const t = useT();
	const speed = useDocumentState((s) =>
		speedLabel(s.document?.items[id]?.speed),
	);
	const off = useDocumentState((s) => s.document?.items[id]?.disabled === true);
	const slot = useDocumentState((s) => s.document?.items[id]?.slot);
	const link = useDocumentState((s) => s.document?.items[id]?.link);
	const missing = useDocumentState((s) => {
		const asset = s.document?.items[id]?.asset;
		return (
			asset !== undefined &&
			(asset === PLACEHOLDER_ASSET || s.document?.assets[asset] === undefined)
		);
	});
	if (
		speed === null &&
		slot === undefined &&
		!missing &&
		!off &&
		link === undefined
	) {
		return null;
	}
	return (
		<span className="relative z-10 ml-1 flex shrink-0 items-center gap-1">
			{speed !== null && (
				<span
					data-badge="speed"
					className="rounded bg-background/80 px-1 text-[10px] leading-4 text-foreground"
					title={t("ui.editor.item.speed_badge", { speed })}
				>
					{speed}
				</span>
			)}
			{link !== undefined && (
				<span
					data-badge="link"
					data-link-id={link}
					role="img"
					aria-label={t("ui.editor.item.link_badge")}
					title={t("ui.editor.item.link_badge")}
				>
					<Link2 aria-hidden="true" className="size-3.5" />
				</span>
			)}
			{off && (
				<span
					data-badge="disabled"
					role="img"
					aria-label={t("ui.editor.item.disabled_badge")}
					title={t("ui.editor.item.disabled_badge")}
				>
					<EyeOff aria-hidden="true" className="size-3.5" />
				</span>
			)}
			{slot !== undefined && (
				<span
					data-badge="slot"
					role="img"
					aria-label={t("ui.editor.item.slot_badge", { name: slot.name })}
					title={slot.name}
				>
					<Puzzle aria-hidden="true" className="size-3.5" />
					<UntrustedText className="sr-only">{slot.name}</UntrustedText>
				</span>
			)}
			{missing && (
				<span
					data-badge="missing-asset"
					role="img"
					aria-label={t("ui.editor.item.asset_missing")}
					title={t("ui.editor.item.asset_missing")}
				>
					<TriangleAlert
						aria-hidden="true"
						className="size-3.5 text-destructive"
					/>
				</span>
			)}
		</span>
	);
}
