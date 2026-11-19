//! A minimal executor for the two futures of the device set-up and the buffer read-back.

use std::future::Future;
use std::pin::pin;
use std::sync::Arc;
use std::task::{Context, Poll, Wake, Waker};
use std::thread::{self, Thread};

struct Unpark(Thread);

impl Wake for Unpark {
	fn wake(self: Arc<Self>) {
		self.0.unpark();
	}
}

/// Runs a future to completion on the calling thread.
pub fn block_on<F: Future>(f: F) -> F::Output {
	let mut f = pin!(f);
	let waker = Waker::from(Arc::new(Unpark(thread::current())));
	let mut cx = Context::from_waker(&waker);
	loop {
		if let Poll::Ready(v) = f.as_mut().poll(&mut cx) {
			return v;
		}
		thread::park();
	}
}
