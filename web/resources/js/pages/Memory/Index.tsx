import type { ReactNode } from "react";

import LibraryPage from "../Library/LibraryPage";

export default function Index(): ReactNode {
	return <LibraryPage kind="memory" />;
}
