// Feature detection of SDD-012 section 21.2. No eval, no external request, no
// blob: URL. Every test returns { id, required, pass, detail, measured }.
"use strict";

const violations = [];
document.addEventListener("securitypolicyviolation", (event) => {
	violations.push(`${event.violatedDirective} ${event.blockedURI}`);
});

const FIXTURE = "fixtures/proxy.mp4";
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const within = (promise, ms, what) =>
	Promise.race([
		promise,
		sleep(ms).then(() => {
			throw new Error(`timeout: ${what}`);
		}),
	]);

function once(target, name) {
	return new Promise((resolve) => target.addEventListener(name, resolve, { once: true }));
}

function result(id, required, pass, detail = "", measured = null) {
	return { id, required, pass, detail, measured };
}

// A fresh muted video on the proxy fixture, ready to play.
async function loadedVideo(fresh = false) {
	// An element can have one MediaElementSourceNode for its whole life, so the
	// audio test takes its own.
	const video = fresh ? document.createElement("video") : document.getElementById("media");
	video.muted = true;
	video.src = FIXTURE;
	video.load();
	await within(once(video, "canplay"), 8000, "canplay");
	return video;
}

const tests = [];

tests.push(async () => {
	const missing = [];
	if (typeof structuredClone !== "function") missing.push("structuredClone");
	if (typeof Array.prototype.at !== "function") missing.push("Array.prototype.at");
	if (typeof Object.hasOwn !== "function") missing.push("Object.hasOwn");
	if (typeof AbortSignal.timeout !== "function") missing.push("AbortSignal.timeout");
	return result("es2022", true, missing.length === 0, missing.length ? `missing ${missing.join(", ")}` : "all present");
});

tests.push(async () => {
	const ok = typeof Intl.RelativeTimeFormat === "function" && typeof Intl.PluralRules === "function";
	if (!ok) return result("intl", true, false, "Intl.RelativeTimeFormat or Intl.PluralRules is missing");
	const relative = new Intl.RelativeTimeFormat("en").format(-1, "minute");
	const plural = new Intl.PluralRules("en").select(1);
	return result("intl", true, relative.length > 0 && plural === "one", `${relative}, ${plural}`);
});

tests.push(async () => {
	try {
		const granted = await within(
			navigator.locks.request("vv-detect", async () => true),
			2000,
			"locks",
		);
		const a = new BroadcastChannel("vv-detect");
		const b = new BroadcastChannel("vv-detect");
		const got = new Promise((resolve) => {
			b.onmessage = (event) => resolve(event.data);
		});
		a.postMessage("hello");
		const message = await within(got, 2000, "broadcast");
		a.close();
		b.close();
		return result("tab-relay", false, granted === true && message === "hello", "lock granted, message delivered");
	} catch (error) {
		return result("tab-relay", false, false, String(error));
	}
});

tests.push(async () => {
	const checks = {
		oklch: CSS.supports("color", "oklch(0.5 0.1 200)"),
		colorMix: CSS.supports("color", "color-mix(in oklab, red, blue)"),
		aspectRatio: CSS.supports("aspect-ratio", "1 / 1"),
		focusVisible: CSS.supports("selector(:focus-visible)"),
	};
	const failed = Object.keys(checks).filter((k) => !checks[k]);
	return result("css-oklch", true, failed.length === 0, failed.length ? `unsupported ${failed.join(", ")}` : "all supported");
});

tests.push(async () => {
	const response = await fetch(FIXTURE, { headers: { Range: "bytes=0-99" } });
	const bytes = (await response.arrayBuffer()).byteLength;
	return result("fetch-range", true, response.status === 206 && bytes === 100, `status ${response.status}, ${bytes} bytes`);
});

tests.push(async () => {
	const vp9 = document.createElement("video").canPlayType('video/mp4; codecs="vp09.00.10.08, mp4a.40.2"');
	try {
		const video = await loadedVideo();
		await within(video.play(), 3000, "play");
		await sleep(2000);
		const time = video.currentTime;
		video.pause();
		return result("video-playback", true, time >= 1.5, `currentTime ${time.toFixed(2)} after 2 s`, {
			current_time: time,
			vp9_aac_can_play_type: vp9,
		});
	} catch (error) {
		return result("video-playback", true, false, String(error), { vp9_aac_can_play_type: vp9 });
	}
});

tests.push(async () => {
	try {
		const video = await loadedVideo();
		const times = [];
		for (let i = 0; i < 20; i++) {
			const target = Math.random() * (video.duration - 0.2);
			const start = performance.now();
			const seeked = once(video, "seeked");
			video.currentTime = target;
			await within(seeked, 3000, "seeked");
			times.push(performance.now() - start);
		}
		times.sort((a, b) => a - b);
		const p50 = times[Math.floor(times.length * 0.5)];
		const p95 = times[Math.floor(times.length * 0.95)];
		return result("video-seek", true, times.length === 20, `p50 ${p50.toFixed(0)} ms, p95 ${p95.toFixed(0)} ms (NFR-003: 100 ms)`, {
			p50_ms: p50,
			p95_ms: p95,
		});
	} catch (error) {
		return result("video-seek", true, false, String(error));
	}
});

tests.push(async () => {
	try {
		const video = await loadedVideo();
		const accepted = [];
		for (const rate of [0.25, 2, 4]) {
			video.playbackRate = rate;
			accepted.push(video.playbackRate === rate);
		}
		const pitch = video.preservesPitch === true;
		return result("video-rate", true, accepted.every(Boolean) && pitch, `rates ${accepted.join("/")}, preservesPitch ${pitch}`);
	} catch (error) {
		return result("video-rate", true, false, String(error));
	}
});

tests.push(async () => {
	try {
		const video = await loadedVideo();
		const seeked = once(video, "seeked");
		video.currentTime = 1;
		await within(seeked, 3000, "seeked");

		// The expected colour: the same frame drawn by a 2D canvas.
		const flat = document.createElement("canvas");
		flat.width = 4;
		flat.height = 4;
		const context = flat.getContext("2d", { willReadFrequently: true });
		context.drawImage(video, 0, 0, 4, 4);
		const expected = [0, 0, 0];
		const data = context.getImageData(0, 0, 4, 4).data;
		for (let i = 0; i < 16; i++) for (let c = 0; c < 3; c++) expected[c] += data[i * 4 + c] / 16;

		const canvas = document.createElement("canvas");
		canvas.width = 4;
		canvas.height = 4;
		const gl = canvas.getContext("webgl2");
		if (!gl) return result("webgl2", true, false, "no WebGL2 context");
		const compile = (type, source) => {
			const shader = gl.createShader(type);
			gl.shaderSource(shader, source);
			gl.compileShader(shader);
			return shader;
		};
		const program = gl.createProgram();
		gl.attachShader(program, compile(gl.VERTEX_SHADER, "#version 300 es\nout vec2 uv;void main(){vec2 p=vec2(float((gl_VertexID<<1)&2),float(gl_VertexID&2));uv=vec2(p.x,1.0-p.y);gl_Position=vec4(p*2.0-1.0,0.0,1.0);}"));
		gl.attachShader(program, compile(gl.FRAGMENT_SHADER, "#version 300 es\nprecision mediump float;in vec2 uv;uniform sampler2D t;out vec4 c;void main(){c=texture(t,uv);}"));
		gl.linkProgram(program);
		gl.useProgram(program);
		const texture = gl.createTexture();
		gl.bindTexture(gl.TEXTURE_2D, texture);
		gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
		gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
		gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
		gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, video);
		gl.viewport(0, 0, 4, 4);
		gl.drawArrays(gl.TRIANGLES, 0, 3);
		const pixels = new Uint8Array(4 * 4 * 4);
		gl.readPixels(0, 0, 4, 4, gl.RGBA, gl.UNSIGNED_BYTE, pixels);
		const mean = [0, 0, 0];
		for (let i = 0; i < 16; i++) for (let c = 0; c < 3; c++) mean[c] += pixels[i * 4 + c] / 16;
		const worst = Math.max(...mean.map((v, c) => Math.abs(v - expected[c])));
		return result("webgl2", true, worst <= 8, `mean colour differs by ${worst.toFixed(1)} per channel`, {
			expected: expected.map(Math.round),
			drawn: mean.map(Math.round),
		});
	} catch (error) {
		return result("webgl2", true, false, String(error));
	}
});

tests.push(async () => {
	try {
		// A 1x1 PNG, written as data so that no URL of another kind is needed.
		const base64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==";
		const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
		const bitmap = await createImageBitmap(new Blob([bytes], { type: "image/png" }));
		const image = new Image();
		image.src = `data:image/png;base64,${base64}`;
		await image.decode();
		return result("image-bitmap", true, bitmap.width === 1 && image.naturalWidth === 1, "bitmap and decode ok");
	} catch (error) {
		return result("image-bitmap", true, false, String(error));
	}
});

// Reads events from /events: the server cuts the first connection after 40 and
// the browser reconnects with Last-Event-ID 40.
tests.push(async () => {
	try {
		const ids = [];
		let resumed = null;
		await within(
			new Promise((resolve) => {
				const source = new EventSource("events");
				source.onmessage = (event) => {
					const data = JSON.parse(event.data);
					ids.push(data.id);
					if (data.id === 41) resumed = data.resumed_from;
					if (data.id === 100) {
						source.close();
						resolve();
					}
				};
			}),
			15000,
			"events",
		);
		const ordered = ids.length === 100 && ids.every((id, i) => id === i + 1);
		return result("eventsource", true, ordered && resumed === "40", `${ids.length} events in order: ${ordered}, resumed from ${resumed}`);
	} catch (error) {
		return result("eventsource", true, false, String(error));
	}
});

let boxMoves = 0;
const box = document.getElementById("box");
box.addEventListener("pointerdown", (event) => {
	box.setPointerCapture(event.pointerId);
});
box.addEventListener("pointermove", (event) => {
	if (box.hasPointerCapture(event.pointerId)) {
		boxMoves++;
		document.getElementById("box-moves").textContent = `${boxMoves} moves captured`;
	}
});

tests.push(async () => {
	let synthetic = false;
	let detail = "";
	try {
		const target = document.createElement("div");
		document.body.append(target);
		const seen = [];
		target.addEventListener("pointerdown", (event) => seen.push(event.type));
		target.dispatchEvent(new PointerEvent("pointerdown", { pointerId: 7, bubbles: true }));
		try {
			target.setPointerCapture(7);
		} catch (error) {
			// An unknown pointer id is refused: capture is proved by the manual step.
			detail = `synthetic capture refused (${error.name}); `;
		}
		synthetic = seen.length === 1;
		target.remove();
	} catch (error) {
		detail = String(error);
	}
	const manual = boxMoves >= 3;
	return result("pointer", true, synthetic && manual, `${detail}${boxMoves} real moves captured`, { manual_moves: boxMoves });
});

tests.push(async () => {
	try {
		const video = await loadedVideo(true);
		const context = new AudioContext();
		const source = context.createMediaElementSource(video);
		const gain = context.createGain();
		gain.gain.value = 4;
		source.connect(gain).connect(context.destination);
		await context.resume().catch(() => undefined);
		const state = context.state;
		const accepted = gain.gain.value === 4;
		return result("audio-graph", true, accepted && state === "running", `gain 4 ${accepted ? "accepted" : "refused"}, state ${state}`);
	} catch (error) {
		return result("audio-graph", true, false, String(error));
	}
});

tests.push(async () => {
	try {
		const element = document.createElement("div");
		element.style.width = "10px";
		element.style.height = "10px";
		document.body.append(element);
		const fired = new Promise((resolve) => {
			let first = true;
			new ResizeObserver(() => {
				if (first) {
					first = false;
					return;
				}
				resolve(true);
			}).observe(element);
		});
		await sleep(50);
		element.style.width = "40px";
		const ok = await within(fired, 2000, "resize");
		element.remove();
		return result("resize-observer", true, ok === true, "observer fired on resize");
	} catch (error) {
		return result("resize-observer", true, false, String(error));
	}
});

tests.push(async () => {
	const id = crypto.randomUUID();
	return result("crypto-uuid", true, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id), id);
});

tests.push(async () => {
	try {
		localStorage.setItem("vv-detect", "1");
		const ok = localStorage.getItem("vv-detect") === "1";
		localStorage.removeItem("vv-detect");
		return result("storage", false, ok, "localStorage write and read");
	} catch (error) {
		return result("storage", false, false, String(error));
	}
});

tests.push(async () => {
	const present = typeof HTMLMediaElement.prototype.fastSeek === "function";
	return result("fast-seek", false, present, present ? "fastSeek exists" : "fastSeek is missing");
});

tests.push(async () => {
	const response = await fetch(".", { cache: "no-store" });
	const policy = response.headers.get("content-security-policy") ?? "";
	await sleep(100);
	const strict = policy !== "" && !policy.includes("unsafe-inline") && !policy.includes("unsafe-eval");
	return result("csp", true, strict && violations.length === 0, `${violations.length} violations; policy ${policy ? "sent" : "missing"}`, {
		violations,
	});
});

let last = null;

function render(results, verdict) {
	const rows = document.getElementById("rows");
	rows.replaceChildren();
	for (const r of results) {
		const row = document.createElement("tr");
		const cells = [r.id, r.required ? "yes" : "no", r.pass ? "pass" : r.required ? "FAIL" : "no", r.detail, r.measured ? JSON.stringify(r.measured) : ""];
		cells.forEach((text, index) => {
			const cell = document.createElement("td");
			cell.textContent = text;
			if (index === 2) cell.dataset.pass = String(r.pass);
			row.append(cell);
		});
		row.dataset.testId = r.id;
		rows.append(row);
	}
	const el = document.getElementById("verdict");
	el.textContent = verdict;
	el.dataset.verdict = verdict;
}

async function run() {
	violations.length = 0;
	const el = document.getElementById("verdict");
	el.textContent = "Running";
	el.dataset.verdict = "running";
	const results = [];
	for (const test of tests) {
		try {
			results.push(await test());
		} catch (error) {
			results.push(result("error", true, false, String(error)));
		}
		render(results, "running");
	}
	const verdict = results.filter((r) => r.required).every((r) => r.pass) ? "pass" : "fail";
	last = {
		verdict,
		date: new Date().toISOString(),
		platform: navigator.platform || "unknown",
		webview: (navigator.userAgent.match(/(Chrome|Version|Firefox)\/[\d.]+/) ?? ["unknown"])[0].replace("/", "-"),
		user_agent: navigator.userAgent,
		results,
	};
	render(results, verdict);
}

document.getElementById("run").addEventListener("click", () => {
	void run();
});

document.getElementById("send").addEventListener("click", async () => {
	if (!last) return;
	const response = await fetch("results", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(last),
	});
	const body = await response.json().catch(() => ({}));
	document.getElementById("sent").textContent = response.ok ? `Saved ${body.file}` : `Not saved (${response.status})`;
});

document.getElementById("download").addEventListener("click", () => {
	if (!last) return;
	// A data: URL, not a blob: one.
	const link = document.createElement("a");
	link.href = `data:application/json;charset=utf-8,${encodeURIComponent(JSON.stringify(last, null, 2))}`;
	link.download = `detect-${last.platform}-${last.webview}.json`.toLowerCase().replace(/[^a-z0-9.-]+/g, "-");
	link.click();
});

void run();
