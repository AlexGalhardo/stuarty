import type { ReactNode } from "react";

import { useLocale, useT } from "@/lib/i18n";
import type { LibraryAsset } from "../editor/api/types";
import { localizeSize } from "./localizeSize";

// The size of a file as the service wrote it (`size_text`, SDD-011 section 14.9): the page only swaps its decimal mark for the language's. A file
// the service cannot size says so in words, with its state, and is never drawn as 0 B; the state is also in the
// attribute, so a test reads it without colour.
export function AssetSize({
	asset,
	className,
}: {
	asset: Pick<LibraryAsset, "bytes" | "size_text" | "state">;
	className?: string;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const known = asset.bytes !== null && asset.size_text !== "";
	return (
		<span
			data-testid="asset-size"
			data-state={asset.state}
			className={className}
		>
			{known
				? localizeSize(asset.size_text, locale)
				: t("ui.assets.size.unknown")}
			{asset.state !== "ready" && (
				<>
					{" "}
					<span data-testid="asset-state">
						({t(`ui.assets.state.${asset.state}`)})
					</span>
				</>
			)}
		</span>
	);
}
