import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { type OperationGroup, useOperations } from "./operations";
import ToolButton from "./ToolButton";
import { ToolGroup } from "./Toolbar";

const GROUP_LABEL: Record<OperationGroup, string> = {
	history: "ui.image.toolbar.history",
	add: "ui.image.toolbar.add",
	arrange: "ui.image.toolbar.arrange",
	model: "ui.image.toolbar.model",
	view: "ui.image.toolbar.view",
};

// The buttons of one group of operations, each running the operation the palette lists under the same id.
export function OperationTools({
	group,
}: {
	group: OperationGroup;
}): ReactNode {
	const t = useT();
	const operations = useOperations().filter((o) => o.group === group);
	return (
		<ToolGroup label={t(GROUP_LABEL[group])}>
			{operations.map((o) => (
				<ToolButton
					key={o.id}
					icon={o.icon}
					label={o.label}
					shortcut={o.shortcut}
					disabled={o.disabled}
					onClick={o.run}
					testId={o.testId}
				/>
			))}
		</ToolGroup>
	);
}
