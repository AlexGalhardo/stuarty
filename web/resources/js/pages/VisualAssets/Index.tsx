import type { ReactNode } from "react";

import { AssetLibrary } from "@/assetlibrary/AssetLibrary";

// Images and colour tables of the asset library (SDD-011 section 14.6).
export default function VisualAssets(): ReactNode {
	return <AssetLibrary section="visual" />;
}
