import {
	createContext,
	type ReactNode,
	useContext,
	useMemo,
	useState,
} from "react";

// Whether the picker of a workspace image is open. The toolbar button and the empty state of the stage open the
// same dialog, which AddTools draws.
interface AddImage {
	picking: boolean;
	setPicking(open: boolean): void;
}

const AddImageContext = createContext<AddImage | null>(null);

export function AddImageProvider({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	const [picking, setPicking] = useState(false);
	const value = useMemo(() => ({ picking, setPicking }), [picking]);
	return (
		<AddImageContext.Provider value={value}>
			{children}
		</AddImageContext.Provider>
	);
}

export function useAddImage(): AddImage {
	const value = useContext(AddImageContext);
	if (value === null) throw new Error("useAddImage outside AddImageProvider");
	return value;
}
