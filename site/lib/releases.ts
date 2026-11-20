/**
 * The download buttons of the landing pages, read from the latest GitHub release when the site is built. The site
 * rebuilds on every published release (.github/workflows/site.yml), so the buttons follow it. Without network, a
 * rate limit or any other failure, the build falls back to the committed default below and never fails.
 *
 * Asset names come from .config/goreleaser.yaml (stuarty_<version>_<os>_<arch>.zip/.tar.gz and the .deb) and from
 * desktop/packaging/ (stuarty-desktop-<version with dashes>.exe, -<arch>.dmg and .AppImage). When the release has
 * no installer for a system, its button opens the release page.
 */

export type Os = "windows" | "macos" | "linux";

export type Download = { os: Os; label: string; href: string; direct: boolean };

export type LatestRelease = { version: string; tag: string; url: string; downloads: Download[] };

const REPO = "AlexGalhardo/stuarty";
const LATEST_PAGE = `https://github.com/${REPO}/releases/latest`;

/** The committed default: the newest release when this file was last changed. */
export const FALLBACK_TAG = "v0.42.0";

type Asset = { name: string; browser_download_url: string };

// The desktop installer first, then the server archive of the same system; an amd64 build before arm64.
const PATTERNS: Record<Os, RegExp[]> = {
	windows: [/^stuarty-desktop-.*\.(exe|msi)$/i, /windows.*amd64.*\.zip$/i, /windows.*\.zip$/i],
	macos: [/^stuarty-desktop-.*arm64\.dmg$/i, /\.dmg$/i, /darwin.*arm64.*\.tar\.gz$/i, /darwin.*\.tar\.gz$/i],
	linux: [/\.AppImage$/i, /_linux_amd64\.deb$/i, /\.deb$/i, /linux.*amd64.*\.tar\.gz$/i],
};

const LABELS: Record<Os, string> = { windows: "Download for Windows", macos: "Download for macOS", linux: "Download for Linux" };

/** Picks the asset of each system; exported for tests. */
export function pickDownloads(tag: string, assets: Asset[]): Download[] {
	return (Object.keys(PATTERNS) as Os[]).map((os) => {
		const asset = PATTERNS[os].map((re) => assets.find((a) => re.test(a.name))).find((a) => a !== undefined);
		return asset
			? { os, label: LABELS[os], href: `https://github.com/${REPO}/releases/download/${tag}/${asset.name}`, direct: true }
			: { os, label: LABELS[os], href: LATEST_PAGE, direct: false };
	});
}

function fallback(): LatestRelease {
	return { version: FALLBACK_TAG.replace(/^v/, ""), tag: FALLBACK_TAG, url: LATEST_PAGE, downloads: pickDownloads(FALLBACK_TAG, []) };
}

let cached: Promise<LatestRelease> | undefined;

/** The latest release, fetched once per build. */
export function latestRelease(): Promise<LatestRelease> {
	cached ??= (async () => {
		try {
			const headers: Record<string, string> = { Accept: "application/vnd.github+json", "User-Agent": "stuarty-site-build" };
			const token = process.env.GITHUB_TOKEN;
			if (token) headers.Authorization = `Bearer ${token}`;
			const res = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`, { headers, signal: AbortSignal.timeout(8000) });
			if (!res.ok) return fallback();
			const body = (await res.json()) as { tag_name?: unknown; html_url?: unknown; assets?: unknown };
			if (typeof body.tag_name !== "string") return fallback();
			const assets = Array.isArray(body.assets)
				? body.assets.filter((a): a is Asset => typeof a?.name === "string" && typeof a?.browser_download_url === "string")
				: [];
			return {
				version: body.tag_name.replace(/^v/, ""),
				tag: body.tag_name,
				url: typeof body.html_url === "string" ? body.html_url : LATEST_PAGE,
				downloads: pickDownloads(body.tag_name, assets),
			};
		} catch {
			return fallback();
		}
	})();
	return cached;
}
