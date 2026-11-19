package service

// ViewFrameRequest asks for one frame of the current revision. There is no
// revision member: only the current revision is rendered.
type ViewFrameRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	At         string `json:"at"`                    // a time; required
	MaxTokens  int    `json:"max_tokens,omitempty"`  // default 800; 100 to 1568
	Width      int    `json:"width,omitempty"`       // even; exclusive with MaxTokens
	// Pin, when not 0, must be the current revision (SDD-003 section 6.3). Set
	// by the HTTP adapter from rev; never on the wire of the Service.
	Pin int64 `json:"-"`
}

// ViewSheetRequest asks for a contact sheet of the current revision.
type ViewSheetRequest struct {
	ProjectID  string   `json:"project_id"`
	TimelineID string   `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	Times      []string `json:"times,omitempty"`       // 2 to 16; exclusive with From, Until and Count
	From       string   `json:"from,omitempty"`        // default "0s"
	Until      string   `json:"until,omitempty"`       // inclusive; default the last frame
	Count      int      `json:"count,omitempty"`       // default 9; 2 to 16
	Cols       int      `json:"cols,omitempty"`        // default ceil(sqrt(count))
	MaxTokens  int      `json:"max_tokens,omitempty"`  // default 1500; 100 to 1568
	// Pin, when not 0, must be the current revision (SDD-003 section 6.3). Set
	// by the HTTP adapter from rev; never on the wire of the Service.
	Pin int64 `json:"-"`
}

// ViewCell is one picture of a view: the index is 1-based, row-major.
type ViewCell struct {
	Index int       `json:"index"`
	At    TimeValue `json:"at"`
}

// ViewResult is an image and what it shows.
type ViewResult struct {
	ProjectID   string     `json:"project_id"`
	Revision    int64      `json:"revision"`
	Mode        string     `json:"mode"` // "frame" | "sheet"
	Width       int        `json:"width"`
	Height      int        `json:"height"`
	ImageTokens int        `json:"image_tokens"`
	MIME        string     `json:"mime"`
	Cells       []ViewCell `json:"cells"`
	// Path is the cached file on the machine of the server; a local caller only.
	Path  string `json:"path,omitempty"`
	Image []byte `json:"image_base64,omitempty" bulk:"true"`
	Text  string `json:"text"`
}
