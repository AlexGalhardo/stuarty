import {
	createContext,
	type ReactNode,
	useContext,
	useEffect,
	useState,
} from "react";

import { type Library, loadLibrary } from "../api/library";

const EMPTY: Library = { images: [], luts: [], brand: null, fonts: [] };

const Context = createContext<Library>(EMPTY);

// The images of the workspace, the brand kit and the font families, read once when the editor opens: what the
// inspector and the toolbar offer to put in the document.
export function LibraryProvider({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	const [library, setLibrary] = useState<Library>(EMPTY);

	useEffect(() => {
		let live = true;
		loadLibrary()
			.then((value) => {
				if (live) setLibrary(value);
			})
			.catch(() => undefined);
		return () => {
			live = false;
		};
	}, []);

	return <Context.Provider value={library}>{children}</Context.Provider>;
}

export function useLibrary(): Library {
	return useContext(Context);
}
