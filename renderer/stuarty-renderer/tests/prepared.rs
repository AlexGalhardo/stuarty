//! A prepared layer is the layer drawn whole (SDD-013 section 4.2.2, epic 55).
//!
//! Every case of `renderer/testdata/image-ops/` (every look, every blend mode, the masks and the clips) is
//! drawn with each of its media layers prepared first, stored again as a prepared texture of the same item
//! id and drawn from there. The frame must be the golden frame of the case, byte for byte, whole and in six
//! bands, and a layer moved by whole pixels must give the frame of the layer moved and drawn whole.

use std::fs;

use stuarty_renderer::json::{Value, parse};

#[path = "support/image_ops.rs"]
mod support;

fn write(v: &Value, out: &mut String) {
	match v {
		Value::Null => out.push_str("null"),
		Value::Bool(b) => out.push_str(if *b { "true" } else { "false" }),
		Value::Num(n) => out.push_str(&format!("{n}")),
		Value::Str(s) => out.push_str(&format!("{s:?}")),
		Value::Arr(a) => {
			out.push('[');
			for (i, x) in a.iter().enumerate() {
				if i > 0 {
					out.push(',');
				}
				write(x, out);
			}
			out.push(']');
		}
		Value::Obj(m) => {
			out.push('{');
			for (i, (k, x)) in m.iter().enumerate() {
				if i > 0 {
					out.push(',');
				}
				out.push_str(&format!("{k:?}:"));
				write(x, out);
			}
			out.push('}');
		}
	}
}

fn member<'a>(v: &'a Value, name: &str) -> &'a Value {
	let Value::Obj(m) = v else {
		panic!("not an object")
	};
	&m.iter().find(|(k, _)| k == name).unwrap().1
}

fn with_layers(root: &Value, layers: Vec<Value>) -> String {
	let Value::Obj(top) = root else {
		panic!("input")
	};
	let top = top
		.iter()
		.map(|(k, v)| {
			if k != "plan" {
				return (k.clone(), v.clone());
			}
			let Value::Obj(plan) = v else { panic!("plan") };
			let plan = plan
				.iter()
				.map(|(k, v)| {
					if k == "layers" {
						(k.clone(), Value::Arr(layers.clone()))
					} else {
						(k.clone(), v.clone())
					}
				})
				.collect();
			(k.clone(), Value::Obj(plan))
		})
		.collect();
	let mut s = String::new();
	write(&Value::Obj(top), &mut s);
	s
}

fn layers_of(root: &Value) -> Vec<Value> {
	let Value::Arr(l) = member(member(root, "plan"), "layers") else {
		panic!("layers")
	};
	l.clone()
}

fn shifted(layer: &Value, dx: f64, dy: f64) -> Value {
	let Value::Obj(m) = layer else {
		panic!("layer")
	};
	Value::Obj(
		m.iter()
			.map(|(k, v)| match (k.as_str(), v) {
				("cx", Value::Num(n)) => (k.clone(), Value::Num(n + dx)),
				("cy", Value::Num(n)) => (k.clone(), Value::Num(n + dy)),
				_ => (k.clone(), v.clone()),
			})
			.collect(),
	)
}

fn is_media(layer: &Value) -> bool {
	matches!(member(layer, "kind"), Value::Str(k) if k == "media")
}

fn item(layer: &Value) -> String {
	let Value::Str(s) = member(layer, "item") else {
		panic!("item")
	};
	s.clone()
}

#[test]
fn every_case_prepared_equals_its_golden_frame() {
	let mut prepared_layers = 0;
	for case in support::cases() {
		let (mut r, input, w, h) = support::load(&case);
		let root = parse(&input).unwrap();
		let layers = layers_of(&root);

		let mut blocks = Vec::new();
		for l in layers.iter().filter(|l| is_media(l)) {
			let one = with_layers(&root, vec![l.clone()]);
			let block = r
				.prepare(one.as_bytes())
				.unwrap_or_else(|e| panic!("{}: {e:?}", case.display()));
			if !block.is_empty() {
				blocks.push((item(l), block));
			}
		}
		for (id, block) in &blocks {
			r.put_prepared(id, block).unwrap();
			prepared_layers += 1;
		}

		let want = fs::read(case.with_extension("rgba")).unwrap();
		let mut got = vec![0_u8; w * h * 4];
		r.render(&input, &mut got).unwrap();
		assert!(
			got == want,
			"{}: the prepared frame differs",
			case.display()
		);

		let text = String::from_utf8(input).unwrap();
		let mut banded = vec![0_u8; w * h * 4];
		for b in 0..6 {
			let rows = h / 6;
			let t = text.replacen(
				"\"scale\":1}",
				&format!(
					"\"scale\":1,\"y0\":{},\"y1\":{}}}",
					b * rows,
					(b + 1) * rows
				),
				1,
			);
			r.render(t.as_bytes(), &mut banded).unwrap();
		}
		assert!(
			banded == want,
			"{}: six bands of the prepared frame differ",
			case.display()
		);
	}
	assert!(prepared_layers >= 40, "{prepared_layers} layers prepared");
}

#[test]
fn a_prepared_layer_moved_is_the_layer_moved() {
	for case in support::cases() {
		let (whole, input, w, h) = support::load(&case);
		let (mut prep, _, _, _) = support::load(&case);
		let root = parse(&input).unwrap();
		let layers = layers_of(&root);

		for l in layers.iter().filter(|l| is_media(l)) {
			let one = with_layers(&root, vec![l.clone()]);
			let block = prep.prepare(one.as_bytes()).unwrap();
			if !block.is_empty() {
				prep.put_prepared(&item(l), &block).unwrap();
			}
		}

		for (dx, dy) in [(3.0, -2.0), (-5.0, 7.0), (0.5, 0.5)] {
			let moved: Vec<Value> = layers.iter().map(|l| shifted(l, dx, dy)).collect();
			let text = with_layers(&root, moved);
			let mut a = vec![0_u8; w * h * 4];
			let mut b = vec![0_u8; w * h * 4];
			whole.render(text.as_bytes(), &mut a).unwrap();
			prep.render(text.as_bytes(), &mut b).unwrap();
			assert!(a == b, "{}: moved by {dx},{dy} differs", case.display());
		}
	}
}
