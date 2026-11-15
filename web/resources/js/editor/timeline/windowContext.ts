import { createContext } from "react";

import type { FrameWindow } from "./virtualize";

// The window of frames the rows mount, or null when every item is mounted.
// Timeline provides a value that changes only when the window does.
export const TimelineWindowContext = createContext<FrameWindow | null>(null);
