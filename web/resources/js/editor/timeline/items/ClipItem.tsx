import type { ReactNode } from "react";
import { stripUrl } from "../../api/urls";
import { fpsOf } from "../../commands/units";
import { useStore } from "../../state/createStore";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import { selectFps } from "../../state/selectors";
import { stripRows, stripTiles } from "./stripTiles";

// The thumbnail strip behind the label of a clip (SDD-012 section 7.4): one
// element per tile of the sprite sheet of the asset, placed and sized by the
// zoom variable of the timeline like the item, so a zoom re-renders nothing.
// The sheet is the browser's background image of the strip route, never a
// blob: URL. Decoration only (section 1.3).
export default function ClipItem({ id }: { id: string }): ReactNode {
	const runtime = useRuntime();
	const item = useDocumentState((s) => s.document?.items[id]);
	const fps = useDocumentState((s) => selectFps(s));
	const strip = useStore(
		runtime.library,
		(s) => s.entries.find((entry) => entry.asset_id === item?.asset)?.strip,
	);
	if (item === undefined || item.asset === undefined || strip === undefined) {
		return null;
	}
	const tiles = stripTiles(strip, fpsOf(fps), item);
	const aspect = strip.tile_w / strip.tile_h;
	const sheet = `url("${stripUrl(runtime.projectId, item.asset)}")`;
	return (
		<span
			aria-hidden="true"
			data-strip=""
			className="pointer-events-none absolute inset-0 overflow-hidden"
			style={{ containerType: "size" }}
		>
			{tiles.map((tile) => (
				<span
					key={tile.index}
					data-strip-tile={tile.index}
					className="absolute inset-y-0 bg-no-repeat"
					style={{
						["--o" as string]: tile.offset,
						["--l" as string]: tile.length,
						left: "calc(var(--o) * var(--ppf) * 1px)",
						width: "calc(var(--l) * var(--ppf) * 1px)",
						backgroundImage: sheet,
						backgroundSize: `calc(${strip.columns * aspect} * 100cqh) ${stripRows(strip) * 100}%`,
						backgroundPosition: `calc(${-tile.column * aspect} * 100cqh) calc(${-tile.row} * 100cqh)`,
					}}
				/>
			))}
		</span>
	);
}
