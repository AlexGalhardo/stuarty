import type { ReactNode } from "react";
import InspectorTabs from "../InspectorTabs";
import {
	ItemAnimation,
	ItemBasics,
	ItemKeyframes,
	ItemVisual,
} from "./ItemSections";
import ShapeSection from "./ShapeSection";

// The shape panel (SDD-012 sections 6.4 and 12.1): Video, Animation and Text (the shape fields).
export default function ShapePanel({ itemId }: { itemId: string }): ReactNode {
	return (
		<InspectorTabs
			tabs={[
				{
					id: "video",
					content: (
						<>
							<ItemBasics itemId={itemId} />
							<ItemVisual itemId={itemId} />
							<ItemKeyframes itemId={itemId} />
						</>
					),
				},
				{ id: "animation", content: <ItemAnimation itemId={itemId} /> },
				{ id: "text", content: <ShapeSection itemId={itemId} /> },
			]}
		/>
	);
}
