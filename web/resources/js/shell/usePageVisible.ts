import { useEffect, useState } from "react";

// Whether this tab is on the screen. The shell holds its event streams only while it is: over HTTP/1.1 a
// browser opens at most six connections to one host, shared by all its tabs, so a stream kept by each hidden
// tab of the studio would leave a new tab, and every request of the others, waiting for a free one. A tab
// that comes back reads again, and a stream opens with what was missed.
export function usePageVisible(): boolean {
	const [visible, setVisible] = useState(true);
	useEffect(() => {
		const read = (): void => setVisible(document.visibilityState !== "hidden");
		read();
		document.addEventListener("visibilitychange", read);
		return () => document.removeEventListener("visibilitychange", read);
	}, []);
	return visible;
}
