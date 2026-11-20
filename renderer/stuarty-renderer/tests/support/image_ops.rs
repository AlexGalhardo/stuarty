//! What the image-operation tests share: the textures the cases draw and the loader of a case (see
//! `image_ops.rs`). The same four patterns are in `engine/renderhost/image_ops_test.go`.

#![allow(dead_code)]

use std::fs;
use std::path::{Path, PathBuf};

use stuarty_renderer::host::{Limits, Renderer};
use stuarty_renderer::json::{Value, parse};

pub const SIDE: usize = 32;

/// The textures of the cases, premultiplied RGBA, `SIDE` by `SIDE`. The same four functions are in
/// `engine/renderhost/image_ops_test.go`.
pub fn pattern(name: &str) -> Vec<u8> {
	let mut pix = Vec::with_capacity(SIDE * SIDE * 4);
	for y in 0..SIDE {
		for x in 0..SIDE {
			let (x, y) = (x as i32, y as i32);
			let px: [i32; 4] = match name {
				"grad" => [x * 8, y * 8, (x + y) * 4, 255],
				"bars" => {
					if x % 8 < 4 {
						[230, 230, 230, 255]
					} else {
						[40, 40, 40, 255]
					}
				}
				"ramp" => {
					let v = (x * 8 + y) & 255;
					[v, v, v, 255]
				}
				"disc" => {
					let d2 = (x - 16) * (x - 16) + (y - 16) * (y - 16);
					let a = if d2 <= 100 {
						255
					} else if d2 <= 144 {
						128
					} else {
						0
					};
					[
						(200 * a + 127) / 255,
						(60 * a + 127) / 255,
						(30 * a + 127) / 255,
						a,
					]
				}
				other => panic!("no pattern {other}"),
			};
			pix.extend(px.iter().map(|&v| v as u8));
		}
	}
	pix
}

pub fn dir() -> PathBuf {
	PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../testdata/image-ops")
}

pub fn items(v: &Value, out: &mut Vec<String>) {
	match v {
		Value::Obj(members) => {
			for (k, m) in members {
				if k == "layers" {
					if let Value::Arr(layers) = m {
						for l in layers {
							if let Value::Obj(f) = l {
								for (k, v) in f {
									if k == "item"
										&& let Value::Str(s) = v
									{
										out.push(s.clone());
									}
								}
							}
						}
					}
				} else {
					items(m, out);
				}
			}
		}
		Value::Arr(a) => a.iter().for_each(|x| items(x, out)),
		_ => {}
	}
}

pub fn output_size(v: &Value) -> (usize, usize) {
	let Value::Obj(top) = v else { panic!("input") };
	for (k, m) in top {
		if k == "output"
			&& let Value::Obj(o) = m
		{
			let n = |name: &str| {
				o.iter()
					.find(|(k, _)| k == name)
					.and_then(|(_, v)| {
						if let Value::Num(n) = v {
							Some(*n as usize)
						} else {
							None
						}
					})
					.unwrap()
			};
			return (n("width"), n("height"));
		}
	}
	panic!("no output");
}

/// A renderer holding the textures of the case, its render input and the size of the frame.
pub fn load(case: &Path) -> (Renderer, Vec<u8>, usize, usize) {
	let input = fs::read(case).unwrap();
	let root = parse(&input).unwrap();
	let mut ids = Vec::new();
	items(&root, &mut ids);
	let (w, h) = output_size(&root);
	let mut r = Renderer::new(Limits::default());
	for id in &ids {
		let name = id.split('#').next().unwrap();
		let pix = pattern(name);
		r.put_texture(id, SIDE, SIDE, &pix, pix.chunks(4).all(|p| p[3] == 255))
			.unwrap();
	}
	(r, input, w, h)
}

/// The cases of the directory, sorted.
pub fn cases() -> Vec<PathBuf> {
	let mut cases: Vec<_> = fs::read_dir(dir())
		.unwrap()
		.map(|e| e.unwrap().path())
		.filter(|p| p.extension().is_some_and(|e| e == "json"))
		.collect();
	cases.sort();
	cases
}
