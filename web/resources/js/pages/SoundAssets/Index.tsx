import type { ReactNode } from "react";

import { AssetLibrary } from "@/assetlibrary/AssetLibrary";

// The sounds of the asset library (SDD-011 section 14.6, D-121): music, effects, narration and generated voices.
export default function SoundAssets(): ReactNode {
	return <AssetLibrary section="audio" />;
}
