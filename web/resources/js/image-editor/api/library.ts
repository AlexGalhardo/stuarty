import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	BrandKit,
	FontFamily,
	LibraryAsset,
} from "../../editor/api/types";
import { API_BASE } from "./imageApi";

// What the screen offers to put in a document: the images of the workspace, the brand kit and the font families.
// Read through the client of the video editor, which already has these calls.
export interface Library {
	images: LibraryAsset[];
	brand: BrandKit | null;
	fonts: FontFamily[];
}

export async function loadLibrary(): Promise<Library> {
	const client = createClient(new HttpTransport(API_BASE));
	const [assets, brand, fonts] = await Promise.allSettled([
		client.listAssets("image"),
		client.getBrandKit(),
		client.getFonts(),
	]);
	return {
		images: assets.status === "fulfilled" ? assets.value.assets : [],
		brand: brand.status === "fulfilled" ? brand.value : null,
		fonts: fonts.status === "fulfilled" ? fonts.value.fonts : [],
	};
}
