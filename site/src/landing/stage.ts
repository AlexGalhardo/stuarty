// The 3D stage of the landing pages (design-system section 11.9): one WebGL renderer per page that draws the living
// background (a full-screen noise shader in the brand colours) and the mascot (the official icon as a clapperboard
// built from primitives). The stage never draws text. It pauses when the tab is hidden or the canvas is off screen,
// caps the pixel ratio, and with reduced motion renders one frame and stops.
import {
	AmbientLight,
	CapsuleGeometry,
	Color,
	DirectionalLight,
	ExtrudeGeometry,
	Group,
	Mesh,
	MeshStandardMaterial,
	OrthographicCamera,
	PerspectiveCamera,
	PlaneGeometry,
	PointLight,
	Scene,
	Shape,
	ShaderMaterial,
	SRGBColorSpace,
	Vector2,
	WebGLRenderer,
	type Uniform,
} from "three";
import { MOTION_EVENT, motionPaused } from "./boot";

/** Where the mascot stands and how it looks, in normalised stage units (x and y from -1 to 1 of the viewport). */
export type MascotPose = {
	x: number;
	y: number;
	scale: number;
	/** Turn around the vertical axis, in radians, added to the pointer follow. */
	turn: number;
	opacity: number;
};

export type Stage = {
	/** Animate-able pose: change its fields (GSAP tweens them) and the next frame draws it. */
	pose: MascotPose;
	/** Snaps the clapper shut and open again: the "action!" of a scene change. */
	clap(): void;
	/** Blinks once. */
	blink(): void;
	/** Adds a short disturbance to the background, for example from the scroll velocity (0 to 1). */
	stir(amount: number): void;
	/** Tints the background flow towards the accent (0) or the indigo (1). */
	setMood(value: number): void;
	/** Draws one frame now (used under reduced motion after a pose change). */
	renderOnce(): void;
	dispose(): void;
};

const INDIGO = "#3534CA";
const LILAC = "#8078FB";
const WHITE = "#FDFDFD";

const vertexShader = /* glsl */ `
	varying vec2 vUv;
	void main() {
		vUv = uv;
		gl_Position = vec4(position.xy, 0.0, 1.0);
	}
`;

// A slow domain-warped value noise: two brand hues flowing over the theme background. Cheap on purpose: three
// octaves, one draw call, at most a tenth of the frame budget on a laptop GPU.
const fragmentShader = /* glsl */ `
	precision highp float;
	varying vec2 vUv;
	uniform float uTime;
	uniform vec2 uResolution;
	uniform vec2 uPointer;
	uniform float uStir;
	uniform float uMood;
	uniform vec3 uBg;
	uniform vec3 uIndigo;
	uniform vec3 uLilac;
	uniform float uDark;

	float hash(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453123); }
	float noise(vec2 p) {
		vec2 i = floor(p);
		vec2 f = fract(p);
		vec2 u = f * f * (3.0 - 2.0 * f);
		return mix(mix(hash(i), hash(i + vec2(1.0, 0.0)), u.x), mix(hash(i + vec2(0.0, 1.0)), hash(i + vec2(1.0, 1.0)), u.x), u.y);
	}
	float fbm(vec2 p) {
		float v = 0.0;
		float a = 0.5;
		for (int i = 0; i < 3; i++) { v += a * noise(p); p *= 2.03; a *= 0.5; }
		return v;
	}

	void main() {
		vec2 uv = vUv;
		float aspect = uResolution.x / max(uResolution.y, 1.0);
		vec2 p = vec2(uv.x * aspect, uv.y) * 1.6;
		float t = uTime * 0.035;
		vec2 toPointer = (uv - uPointer) * vec2(aspect, 1.0);
		float near = exp(-dot(toPointer, toPointer) * 6.0);
		vec2 warp = vec2(fbm(p + t), fbm(p - t + 3.1));
		float n = fbm(p + warp * (1.4 + uStir * 0.8) + near * 0.35);
		float band = smoothstep(0.38, 0.85, n);
		vec3 hue = mix(uLilac, uIndigo, clamp(uMood + (n - 0.5) * 0.8, 0.0, 1.0));
		// Kept low in both themes so that body text over the flow keeps 4.5:1 (light: a pale tint; dark: a dim glow).
		float strength = mix(0.16, 0.3, uDark) * band + near * 0.06;
		vec3 color = mix(uBg, hue, strength);
		// Vignette towards the theme background at the edges.
		float v = smoothstep(1.25, 0.35, length((uv - 0.5) * vec2(aspect, 1.0)));
		color = mix(uBg, color, v);
		gl_FragColor = vec4(color, 1.0);
	}
`;

function roundedRect(w: number, h: number, r: number): Shape {
	const s = new Shape();
	const x = -w / 2;
	const y = -h / 2;
	s.moveTo(x + r, y);
	s.lineTo(x + w - r, y);
	s.quadraticCurveTo(x + w, y, x + w, y + r);
	s.lineTo(x + w, y + h - r);
	s.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
	s.lineTo(x + r, y + h);
	s.quadraticCurveTo(x, y + h, x, y + h - r);
	s.lineTo(x, y + r);
	s.quadraticCurveTo(x, y, x + r, y);
	return s;
}

function slab(w: number, h: number, r: number, depth: number): ExtrudeGeometry {
	const g = new ExtrudeGeometry(roundedRect(w, h, r), {
		depth,
		bevelEnabled: true,
		bevelThickness: 0.06,
		bevelSize: 0.06,
		bevelSegments: 4,
		curveSegments: 10,
	});
	g.translate(0, 0, -depth / 2);
	return g;
}

type Mascot = {
	root: Group;
	arm: Group;
	eyes: Mesh[];
	pupils: Group;
	materials: MeshStandardMaterial[];
};

/** The icon in 3D: a rounded indigo body, the clapper arm hinged at its top left with a lilac stripe, two white eyes. */
function buildMascot(): Mascot {
	const root = new Group();
	const indigo = new MeshStandardMaterial({ color: INDIGO, roughness: 0.42, metalness: 0.08 });
	const lilac = new MeshStandardMaterial({ color: LILAC, roughness: 0.35, metalness: 0.05, emissive: LILAC, emissiveIntensity: 0.15 });
	const white = new MeshStandardMaterial({ color: WHITE, roughness: 0.25, metalness: 0, emissive: WHITE, emissiveIntensity: 0.25 });

	const body = new Mesh(slab(2.3, 1.62, 0.42, 0.62), indigo);
	body.position.set(0, -0.32, 0);
	root.add(body);

	// The arm turns around a hinge at the top-left corner of the body.
	const arm = new Group();
	arm.position.set(-1.08, 0.62, 0);
	const bar = new Mesh(slab(2.32, 0.36, 0.16, 0.5), indigo);
	bar.position.set(1.16, 0.08, 0);
	// The stripe is a parallelogram set into the bar, as on the icon.
	const stripeShape = new Shape();
	stripeShape.moveTo(-0.2, -0.15);
	stripeShape.lineTo(0.04, -0.15);
	stripeShape.lineTo(0.2, 0.15);
	stripeShape.lineTo(-0.04, 0.15);
	stripeShape.lineTo(-0.2, -0.15);
	const stripeGeometry = new ExtrudeGeometry(stripeShape, { depth: 0.62, bevelEnabled: false });
	stripeGeometry.translate(0, 0, -0.31);
	const stripe = new Mesh(stripeGeometry, lilac);
	stripe.position.set(0.72, 0.08, 0);
	arm.add(bar, stripe);
	root.add(arm);

	const pupils = new Group();
	const eyeGeometry = new CapsuleGeometry(0.115, 0.3, 6, 16);
	const eyes = [-0.38, 0.38].map((x) => {
		const eye = new Mesh(eyeGeometry, white);
		eye.position.set(x, -0.4, 0.4);
		pupils.add(eye);
		return eye;
	});
	root.add(pupils);

	return { root, arm, eyes, pupils, materials: [indigo, lilac, white] };
}

function prefersReducedMotion(): boolean {
	return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function isDark(): boolean {
	return document.documentElement.classList.contains("dark");
}

/** Probes WebGL without three: a failure here means the static fallback stays. */
export function webglAvailable(): boolean {
	try {
		const c = document.createElement("canvas");
		return Boolean(c.getContext("webgl2") ?? c.getContext("webgl"));
	} catch {
		return false;
	}
}

const ARM_OPEN = 0.32;

/**
 * Starts the stage on a canvas. Returns null when WebGL is missing or the renderer fails, so the caller keeps the
 * static gradient and the still mascot. `showMascot` false draws only the background.
 */
export function createStage(canvas: HTMLCanvasElement, options: { showMascot?: boolean } = {}): Stage | null {
	if (!webglAvailable()) return null;
	let renderer: WebGLRenderer;
	try {
		renderer = new WebGLRenderer({ canvas, antialias: true, alpha: false, powerPreference: "low-power" });
	} catch {
		return null;
	}
	renderer.outputColorSpace = SRGBColorSpace;
	renderer.autoClear = false;

	const reduced = prefersReducedMotion();
	// Reduced motion, or the visitor paused the animations: no loop, no clap, no blink; the last frame stays.
	const frozen = (): boolean => reduced || motionPaused();
	const pose: MascotPose = { x: 0, y: 0, scale: 1, turn: 0, opacity: options.showMascot === false ? 0 : 1 };

	// Background: a full-screen triangle pair in clip space.
	const bgScene = new Scene();
	const bgCamera = new OrthographicCamera(-1, 1, 1, -1, 0, 1);
	const uniforms: Record<string, Uniform<unknown>> = {
		uTime: { value: 0 },
		uResolution: { value: new Vector2(1, 1) },
		uPointer: { value: new Vector2(0.5, 0.5) },
		uStir: { value: 0 },
		uMood: { value: 0.35 },
		uBg: { value: new Color("#08090c") },
		uIndigo: { value: new Color(INDIGO) },
		uLilac: { value: new Color(LILAC) },
		uDark: { value: 1 },
	};
	const bgMaterial = new ShaderMaterial({ uniforms, vertexShader, fragmentShader, depthWrite: false, depthTest: false });
	const bgGeometry = new PlaneGeometry(2, 2);
	bgScene.add(new Mesh(bgGeometry, bgMaterial));

	// Mascot scene.
	const scene = new Scene();
	const camera = new PerspectiveCamera(32, 1, 0.1, 100);
	camera.position.set(0, 0, 11);
	scene.add(new AmbientLight("#ffffff", 0.9));
	const key = new DirectionalLight("#ffffff", 2.1);
	key.position.set(3, 4, 6);
	const rim = new PointLight(LILAC, 18, 20, 2);
	rim.position.set(-4, 2, -2);
	scene.add(key, rim);
	const mascot = buildMascot();
	scene.add(mascot.root);
	mascot.arm.rotation.z = ARM_OPEN;

	const pointer = { x: 0, y: 0, tx: 0, ty: 0 };
	let stir = 0;
	let clapStart = -1;
	let blinkStart = -1;
	let nextBlink = 2.5;
	let running = false;
	let onScreen = true;
	let raf = 0;
	let last = performance.now();
	let time = 0;

	const applyTheme = (): void => {
		const dark = isDark();
		(uniforms.uBg.value as Color).set(dark ? "#08090c" : "#ffffff");
		uniforms.uDark.value = dark ? 1 : 0;
		renderer.setClearColor(dark ? "#08090c" : "#ffffff", 1);
	};

	const resize = (): void => {
		const w = canvas.clientWidth || window.innerWidth;
		const h = canvas.clientHeight || window.innerHeight;
		const cap = w < 768 ? 1.5 : 2;
		renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, cap));
		renderer.setSize(w, h, false);
		camera.aspect = w / h;
		camera.updateProjectionMatrix();
		(uniforms.uResolution.value as Vector2).set(w, h);
	};

	// Half-height of the view at the mascot's depth, to map normalised pose units to world units.
	const halfHeight = (): number => Math.tan((camera.fov * Math.PI) / 360) * camera.position.z;

	const draw = (): void => {
		uniforms.uTime.value = time;
		uniforms.uStir.value = stir;
		renderer.clear();
		renderer.render(bgScene, bgCamera);
		if (pose.opacity > 0.01) {
			const hh = halfHeight();
			const hw = hh * camera.aspect;
			const root = mascot.root;
			root.visible = true;
			root.position.set(pose.x * hw, pose.y * hh + Math.sin(time * 1.3) * 0.06, 0);
			const s = pose.scale * Math.min(1, camera.aspect / 0.9);
			root.scale.set(s, s, s);
			root.rotation.set(-0.12 - pointer.y * 0.25, pose.turn + pointer.x * 0.45, Math.sin(time * 0.7) * 0.02);
			mascot.pupils.position.set(pointer.x * 0.08, pointer.y * 0.05, 0);
			for (const m of mascot.materials) {
				m.transparent = pose.opacity < 0.99;
				m.opacity = pose.opacity;
			}
			renderer.clearDepth();
			renderer.render(scene, camera);
		} else {
			mascot.root.visible = false;
		}
	};

	const step = (now: number): void => {
		const dt = Math.min(0.05, (now - last) / 1000);
		last = now;
		time += dt;
		pointer.x += (pointer.tx - pointer.x) * Math.min(1, dt * 4);
		pointer.y += (pointer.ty - pointer.y) * Math.min(1, dt * 4);
		(uniforms.uPointer.value as Vector2).set(0.5 + pointer.x * 0.5, 0.5 + pointer.y * 0.5);
		stir += (0 - stir) * Math.min(1, dt * 1.5);

		// Clapper: shut fast (90 ms), open with an ease (350 ms).
		if (clapStart >= 0) {
			const t = time - clapStart;
			if (t < 0.09) mascot.arm.rotation.z = ARM_OPEN * (1 - t / 0.09);
			else if (t < 0.44) {
				const k = (t - 0.09) / 0.35;
				mascot.arm.rotation.z = ARM_OPEN * (1 - Math.pow(1 - k, 3));
			} else {
				mascot.arm.rotation.z = ARM_OPEN;
				clapStart = -1;
			}
		}
		// Blink: eyes squash for 140 ms, every few seconds.
		if (blinkStart < 0 && time > nextBlink) blinkStart = time;
		if (blinkStart >= 0) {
			const t = (time - blinkStart) / 0.14;
			const sy = t < 1 ? 1 - Math.sin(Math.PI * t) * 0.9 : 1;
			for (const e of mascot.eyes) e.scale.set(1, sy, 1);
			if (t >= 1) {
				blinkStart = -1;
				nextBlink = time + 2.5 + Math.random() * 3.5;
			}
		}
		draw();
		raf = requestAnimationFrame(step);
	};

	const start = (): void => {
		if (running || frozen() || !onScreen || document.hidden) return;
		running = true;
		last = performance.now();
		raf = requestAnimationFrame(step);
	};
	const stop = (): void => {
		running = false;
		cancelAnimationFrame(raf);
	};

	const onPointer = (e: PointerEvent): void => {
		pointer.tx = (e.clientX / window.innerWidth) * 2 - 1;
		pointer.ty = -((e.clientY / window.innerHeight) * 2 - 1);
	};
	const onVisibility = (): void => (document.hidden ? stop() : start());
	const onResize = (): void => {
		resize();
		if (!running) draw();
	};
	const themeObserver = new MutationObserver(() => {
		applyTheme();
		if (!running) draw();
	});
	themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
	const io = new IntersectionObserver((entries) => {
		onScreen = entries.some((e) => e.isIntersecting);
		if (onScreen) start();
		else stop();
	});
	io.observe(canvas);

	const onMotion = (): void => (frozen() ? stop() : start());
	document.addEventListener(MOTION_EVENT, onMotion);
	window.addEventListener("pointermove", onPointer, { passive: true });
	document.addEventListener("visibilitychange", onVisibility);
	window.addEventListener("resize", onResize);

	applyTheme();
	resize();
	draw();
	canvas.dataset.ready = "true";
	start();

	return {
		pose,
		clap() {
			if (frozen()) return;
			clapStart = time;
		},
		blink() {
			if (frozen()) return;
			blinkStart = time;
		},
		stir(amount: number) {
			if (frozen()) return;
			stir = Math.min(1, Math.max(stir, amount));
		},
		setMood(value: number) {
			uniforms.uMood.value = value;
			if (!running) draw();
		},
		renderOnce() {
			if (!running) draw();
		},
		dispose() {
			stop();
			io.disconnect();
			themeObserver.disconnect();
			window.removeEventListener("pointermove", onPointer);
			document.removeEventListener("visibilitychange", onVisibility);
			document.removeEventListener(MOTION_EVENT, onMotion);
			window.removeEventListener("resize", onResize);
			bgGeometry.dispose();
			bgMaterial.dispose();
			for (const m of mascot.materials) m.dispose();
			renderer.dispose();
		},
	};
}
