//! Windows Graphics Capture. Written in task 6 of epic 61.

use std::sync::Arc;

use crate::frame::Mailbox;
use crate::protocol::{SourceInfo, StartRequest};
use crate::source::{RunningSource, SourceError};

pub fn open(
	_req: &StartRequest,
	_mailbox: &Arc<Mailbox>,
) -> Result<Box<dyn RunningSource>, SourceError> {
	Err(SourceError::new(
		"unsupported",
		"Windows capture is not built yet",
	))
}

pub fn list() -> (Vec<SourceInfo>, Vec<SourceInfo>) {
	(Vec::new(), Vec::new())
}
