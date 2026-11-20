// The calendar of publications as the service answers it (SDD-020 sections 23 to 29). Every state, time and
// error here is read from the answer: the page draws them and computes none.

export const TARGET_STATES = [
	"draft",
	"waiting_approval",
	"approved",
	"scheduled",
	"publishing",
	"published",
	"sent_as_draft",
	"failed",
	"missed",
	"cancelled",
] as const;

export type TargetState = (typeof TARGET_STATES)[number];

export interface TargetError {
	code: string;
	kind?: string;
	// Written by the service from a kind and a reason code; the text of a network is never in it.
	message: string;
	hint?: string;
	retryable: boolean;
}

export interface TargetFields {
	title?: string;
	description?: string;
	tags?: string[];
	visibility?: string;
	made_for_kids?: boolean;
	ai_disclosure?: boolean;
}

export interface Target {
	id: string;
	publication_id: string;
	connection_id: string;
	network: string;
	fields: TargetFields;
	state: TargetState;
	// The instant in UTC, and the zone the person chose; local_time is the instant on the wall clock of that zone.
	scheduled_at: string | null;
	zone: string;
	local_time?: string;
	late_window_minutes: number;
	send_now?: boolean;
	approval_id?: string;
	attempts: number;
	next_attempt_at?: string;
	remote_id?: string;
	url?: string;
	remote_state?: string;
	error?: TargetError;
	published_at?: string;
	updated_at: string;
}

export interface Publication {
	id: string;
	project_id: string;
	title: string;
	text?: string;
	export: string;
	thumbnail?: string;
	// The id of the text of a blog post, instead of an export (SDD-020 section 43).
	post?: string;
	targets: Target[];
	notes?: string[];
	created_at: string;
	updated_at: string;
}

export interface PublicationList {
	publications: Publication[];
	total: number;
}

// What the editor sends for one target; an id changes an existing one, no id adds one.
export interface TargetInput {
	id?: string;
	connection_id?: string;
	fields?: TargetFields;
	at?: string;
	zone?: string;
	late_window_minutes?: number;
	remove?: boolean;
}

export interface PublicationCreateInput {
	project_id: string;
	title: string;
	text?: string;
	export?: string;
	thumbnail?: string;
	post?: string;
	targets?: TargetInput[];
}

export interface PublicationUpdateInput {
	title?: string;
	text?: string;
	export?: string;
	thumbnail?: string;
	post?: string;
	targets?: TargetInput[];
}

export interface ScheduleInput {
	target_id?: string;
	at?: string;
	zone?: string;
	late_window_minutes?: number;
}

export interface ListFilter {
	project_id?: string;
	state?: string;
	network?: string;
	from?: string;
	to?: string;
	limit?: number;
	offset?: number;
}

// The events of GET /publications/events: the data of each is a Publication.
export const PUBLICATION_EVENT = "publication.updated";
