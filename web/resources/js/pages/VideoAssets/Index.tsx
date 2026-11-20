import type { ReactNode } from "react";

import { AssetLibrary } from "@/assetlibrary/AssetLibrary";

// Video and audio of the asset library (SDD-011 section 14.6).
export default function VideoAssets(): ReactNode {
	return <AssetLibrary section="video" />;
}
