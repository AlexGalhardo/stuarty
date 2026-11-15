import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type { Browser, CDPSession, Page } from "@playwright/test";

const run = promisify(execFile);

// The memory readings of the soak (SDD-012 section 19.1, NFR-008).

export interface BrowserProcess {
	pid: number;
	// "browser", "renderer", "GPU", "utility" and so on, as Chromium names them.
	type: string;
}

// The processes of the launched browser, by process id, from the browser
// itself (CDP SystemInfo.getProcessInfo). Chromium only.
export async function browserProcesses(
	browser: Browser,
): Promise<BrowserProcess[]> {
	const session = await browser.newBrowserCDPSession();
	try {
		const info = (await session.send("SystemInfo.getProcessInfo")) as {
			processInfo: { type: string; id: number }[];
		};
		return info.processInfo.map((p) => ({ pid: p.id, type: p.type }));
	} finally {
		await session.detach();
	}
}

// Resident memory in bytes of each process id, read from the operating system:
// the working set on Windows, the resident set elsewhere. A process that has
// gone is absent from the result.
export async function residentBytes(
	pids: readonly number[],
): Promise<Map<number, number>> {
	const out = new Map<number, number>();
	if (pids.length === 0) return out;
	if (process.platform === "win32") {
		const { stdout } = await run("powershell.exe", [
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			`Get-Process -Id ${pids.join(",")} -ErrorAction SilentlyContinue | ForEach-Object { "$($_.Id) $($_.WorkingSet64)" }`,
		]);
		for (const line of stdout.split(/\r?\n/)) {
			const [pid, bytes] = line.trim().split(/\s+/).map(Number);
			if (Number.isFinite(pid) && Number.isFinite(bytes)) out.set(pid, bytes);
		}
		return out;
	}
	// `ps` exits with 1 when some of the ids are gone; it still prints the rest.
	const { stdout } = await run("ps", [
		"-o",
		"pid=,rss=",
		"-p",
		pids.join(","),
	]).catch((error: { stdout?: string }) => ({ stdout: error.stdout ?? "" }));
	for (const line of stdout.split(/\r?\n/)) {
		const [pid, kilobytes] = line.trim().split(/\s+/).map(Number);
		if (Number.isFinite(pid) && Number.isFinite(kilobytes)) {
			out.set(pid, kilobytes * 1024);
		}
	}
	return out;
}

export interface MemorySample {
	jsHeapBytes: number;
	// Renderer plus GPU processes (the tab of NFR-008) and the whole tree.
	tabBytes: number;
	treeBytes: number;
	domNodes: number;
	listeners: number;
	documents: number;
}

// One reading after a forced garbage collection. The page must have been
// launched with `--enable-precise-memory-info --js-flags=--expose-gc`, or the
// heap number is rounded and the collection is a no-op.
export async function sampleMemory(
	browser: Browser,
	page: Page,
	session: CDPSession,
): Promise<MemorySample> {
	await session.send("HeapProfiler.collectGarbage");
	await page.evaluate(() => {
		(window as unknown as { gc?: () => void }).gc?.();
	});
	const jsHeapBytes = await page.evaluate(
		() =>
			(performance as unknown as { memory: { usedJSHeapSize: number } }).memory
				.usedJSHeapSize,
	);
	const counters = (await session.send("Memory.getDOMCounters")) as {
		documents: number;
		nodes: number;
		jsEventListeners: number;
	};
	const processes = await browserProcesses(browser);
	const resident = await residentBytes(processes.map((p) => p.pid));
	let tabBytes = 0;
	let treeBytes = 0;
	for (const { pid, type } of processes) {
		const bytes = resident.get(pid) ?? 0;
		treeBytes += bytes;
		if (type === "renderer" || type === "GPU") tabBytes += bytes;
	}
	return {
		jsHeapBytes,
		tabBytes,
		treeBytes,
		domNodes: counters.nodes,
		listeners: counters.jsEventListeners,
		documents: counters.documents,
	};
}

export const MB = 1024 * 1024;
