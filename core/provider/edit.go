package provider

import (
	"slices"
	"strconv"
	"strings"
)

// CapabilityImageEdit is the capability of a provider that edits a picture it is sent (SDD-016 section 2.4).
const CapabilityImageEdit = "image_edit"

// The operations of the job image.edit (SDD-016 section 6.5).
const (
	// OperationRemoveBackground sends a picture to the provider and gets it back with a transparent background.
	OperationRemoveBackground = "remove_background"
	// OperationGenerate makes a new image from a prompt, into a document; it uses the capability image.
	OperationGenerate = "generate"
)

// EditOperations are the operations a provider may do to a picture it is sent.
var EditOperations = []string{OperationRemoveBackground}

// DefaultMaxSourceBytes is the largest picture sent when the provider documents no limit: 15 MiB, which keeps the
// base64 form of it inside the 20,971,520 characters OpenAI takes for an image.
const DefaultMaxSourceBytes = 15 << 20

// EditRequest is one request to edit a picture (SDD-016 section 5.5).
type EditRequest struct {
	Model     string `json:"model"`
	Operation string `json:"operation"`
	// Size is "<width>x<height>", or empty for the size the provider chooses.
	Size string `json:"size,omitempty"`
}

// EditLimits are what a provider documents for one model; a zero value is "not documented".
type EditLimits struct {
	// Operations are the operations of EditOperations the model can do.
	Operations []string
	Sizes      []string
	// MaxSourceBytes is the largest picture the provider takes.
	MaxSourceBytes int64
}

// SourceLimit is MaxSourceBytes, or DefaultMaxSourceBytes when the provider documents none.
func (l EditLimits) SourceLimit() int64 {
	if l.MaxSourceBytes > 0 {
		return l.MaxSourceBytes
	}

	return DefaultMaxSourceBytes
}

// NormalizeEdit trims the model, the operation and the size.
func NormalizeEdit(r EditRequest) EditRequest {
	r.Model = strings.TrimSpace(r.Model)
	r.Operation = strings.TrimSpace(r.Operation)
	r.Size = strings.ToLower(strings.TrimSpace(r.Size))

	return r
}

// ValidateEdit checks a request, in stored form (NormalizeEdit), against SDD-016 section 5.5 and the limits the
// adapter documents for the model.
func ValidateEdit(r EditRequest, l EditLimits) *FieldError {
	if !ValidModel(r.Model) {
		return bad("model", "must be 1 to %d characters of letters, digits and . _ : / @ + -", MaxModelID)
	}

	if !slices.Contains(EditOperations, r.Operation) {
		return bad("operation", "must be one of %s", strings.Join(EditOperations, ", "))
	}

	if len(l.Operations) > 0 && !slices.Contains(l.Operations, r.Operation) {
		return bad("operation", "the model does only %s", strings.Join(l.Operations, ", "))
	}

	if r.Size != "" {
		if f := validSize(r.Size, l.Sizes); f != nil {
			return f
		}
	}

	return nil
}

// ClosestSize is the size, among the allowed ones, whose proportions are nearest to those of a box width by
// height; the first of the list wins a tie. With no size documented it is DefaultImageSize. It is whole-number
// arithmetic, so the same box and the same list always give the same size.
func ClosestSize(width, height int, sizes []string) string {
	if width < 1 || height < 1 {
		return DefaultImageSize
	}

	best, bestH := "", 0

	var bestGap int64

	for _, s := range sizes {
		m := sizeRe.FindStringSubmatch(s)
		if m == nil {
			continue
		}

		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])

		if w < 1 || h < 1 {
			continue
		}

		// The gap |width/height - w/h| is gap / (height * h); height is common, so two gaps compare as
		// gapA / hA against gapB / hB, by cross products.
		gap := int64(width)*int64(h) - int64(w)*int64(height)
		if gap < 0 {
			gap = -gap
		}

		if best == "" || gap*int64(bestH) < bestGap*int64(h) {
			best, bestH, bestGap = s, h, gap
		}
	}

	if best == "" {
		return DefaultImageSize
	}

	return best
}
