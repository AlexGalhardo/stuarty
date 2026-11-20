// The subset of three 0.187.0 that the landing pages use. three ships no type declarations and @types/three is not
// an approved dependency, so the classes and members used here are declared by hand; extend this file when a page
// uses more of the library.
declare module "three" {
	export const SRGBColorSpace: string;
	export const DoubleSide: number;

	export class Color {
		constructor(color?: string | number);
		set(color: string | number): this;
		r: number;
		g: number;
		b: number;
	}

	export class Vector2 {
		constructor(x?: number, y?: number);
		x: number;
		y: number;
		set(x: number, y: number): this;
	}

	export class Vector3 {
		constructor(x?: number, y?: number, z?: number);
		x: number;
		y: number;
		z: number;
		set(x: number, y: number, z: number): this;
	}

	export class Euler {
		x: number;
		y: number;
		z: number;
		set(x: number, y: number, z: number): this;
	}

	export class Object3D {
		position: Vector3;
		rotation: Euler;
		scale: Vector3;
		visible: boolean;
		renderOrder: number;
		add(...objects: Object3D[]): this;
		remove(...objects: Object3D[]): this;
		traverse(callback: (object: Object3D) => void): void;
	}

	export class Group extends Object3D {}
	export class Scene extends Object3D {}

	export class Camera extends Object3D {}

	export class PerspectiveCamera extends Camera {
		constructor(fov?: number, aspect?: number, near?: number, far?: number);
		aspect: number;
		fov: number;
		updateProjectionMatrix(): void;
		lookAt(x: number, y: number, z: number): void;
	}

	export class OrthographicCamera extends Camera {
		constructor(left?: number, right?: number, top?: number, bottom?: number, near?: number, far?: number);
	}

	export class BufferGeometry {
		dispose(): void;
		translate(x: number, y: number, z: number): this;
		center(): this;
	}

	export class PlaneGeometry extends BufferGeometry {
		constructor(width?: number, height?: number);
	}

	export class BoxGeometry extends BufferGeometry {
		constructor(width?: number, height?: number, depth?: number);
	}

	export class CapsuleGeometry extends BufferGeometry {
		constructor(radius?: number, height?: number, capSegments?: number, radialSegments?: number);
	}

	export class Path {
		moveTo(x: number, y: number): this;
		lineTo(x: number, y: number): this;
		quadraticCurveTo(cpX: number, cpY: number, x: number, y: number): this;
	}

	export class Shape extends Path {}

	export interface ExtrudeOptions {
		depth?: number;
		bevelEnabled?: boolean;
		bevelThickness?: number;
		bevelSize?: number;
		bevelSegments?: number;
		curveSegments?: number;
	}

	export class ExtrudeGeometry extends BufferGeometry {
		constructor(shape: Shape, options?: ExtrudeOptions);
	}

	export class Material {
		transparent: boolean;
		opacity: number;
		depthWrite: boolean;
		dispose(): void;
	}

	export interface StandardOptions {
		color?: string | number | Color;
		roughness?: number;
		metalness?: number;
		emissive?: string | number | Color;
		emissiveIntensity?: number;
		transparent?: boolean;
		opacity?: number;
	}

	export class MeshStandardMaterial extends Material {
		constructor(options?: StandardOptions);
		color: Color;
		emissive: Color;
	}

	export class MeshBasicMaterial extends Material {
		constructor(options?: { color?: string | number | Color; transparent?: boolean; opacity?: number });
		color: Color;
	}

	export interface Uniform<T> {
		value: T;
	}

	export class ShaderMaterial extends Material {
		constructor(options: {
			uniforms: Record<string, Uniform<unknown>>;
			vertexShader: string;
			fragmentShader: string;
			depthWrite?: boolean;
			depthTest?: boolean;
		});
		uniforms: Record<string, Uniform<unknown>>;
	}

	export class Mesh extends Object3D {
		constructor(geometry?: BufferGeometry, material?: Material);
		geometry: BufferGeometry;
		material: Material;
	}

	export class Light extends Object3D {
		intensity: number;
	}

	export class AmbientLight extends Light {
		constructor(color?: string | number, intensity?: number);
	}

	export class DirectionalLight extends Light {
		constructor(color?: string | number, intensity?: number);
	}

	export class PointLight extends Light {
		constructor(color?: string | number, intensity?: number, distance?: number, decay?: number);
		color: Color;
	}

	export class WebGLRenderer {
		constructor(options?: {
			canvas?: HTMLCanvasElement;
			antialias?: boolean;
			alpha?: boolean;
			powerPreference?: "default" | "high-performance" | "low-power";
		});
		domElement: HTMLCanvasElement;
		outputColorSpace: string;
		autoClear: boolean;
		setPixelRatio(ratio: number): void;
		setSize(width: number, height: number, updateStyle?: boolean): void;
		setClearColor(color: string | number | Color, alpha?: number): void;
		clear(): void;
		clearDepth(): void;
		render(scene: Object3D, camera: Camera): void;
		dispose(): void;
	}
}
