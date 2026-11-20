package untrusted

// Provenance is the table of SDD-007 section 8.1: every field of a service
// result that carries text the product did not generate, by type and field,
// with the source it is delimited as. A test in package service walks the
// results of the Service for fields tagged untrusted:"true" and fails on any
// that is not here, so a new field cannot reach an agent undelimited.
var Provenance = map[string]string{
	"service.ProjectSummary.Name":              SourceProjectText,
	"service.ProjectCreateResult.Name":         SourceProjectText,
	"service.ProjectGetResult.Name":            SourceProjectText,
	"service.ImportedAsset.Name":               SourceAssetName,
	"assets.LibraryAsset.Name":                 SourceAssetName,
	"service.UntrustedData.Metadata":           SourceMetadata,
	"service.TranscriptLine.Text":              SourceTranscript,
	"service.MediaEntry.Name":                  SourceAssetName,
	"service.UntrustedData.Name":               SourceAssetName,
	"service.TemplateSummary.Name":             SourceTemplateText,
	"service.TemplateSummary.Description":      SourceTemplateText,
	"service.SlotInfo.Description":             SourceTemplateText,
	"service.OtioLost.Name":                    SourceProjectText,
	"service.OtioLost.Feature":                 SourceProjectText,
	"service.OtioMatch.Ref":                    SourceAssetName,
	"service.OtioMedia.Ref":                    SourceAssetName,
	"service.OtioMedia.Reason":                 SourceAssetName,
	"service.ProjectOtioExportResult.FileName": SourceProjectText,
	"service.ProjectItem.Title":                SourceProjectText,
	"service.ProjectItem.Text":                 SourceProjectText,
	"service.ProjectDescribeResult.Name":       SourceProjectText,
	"service.BriefResult.Text":                 SourceProjectText,
	"service.BriefingAnswer.Value":             SourceProjectText,

	// image documents (SDD-021): names, texts and the descriptions built from them
	"service.ImageCreateResult.Text":       SourceProjectText,
	"service.ImageListResult.Text":         SourceProjectText,
	"service.ImageSummary.Name":            SourceProjectText,
	"service.ImageDescribeResult.Name":     SourceProjectText,
	"service.ImageDescribeResult.Layers":   SourceProjectText,
	"service.ImageDescribeResult.Text":     SourceProjectText,
	"service.ImageEditResult.Description":  SourceProjectText,
	"connections.Connection.Label":         SourceProjectText,
	"service.ConnectionTestResult.Message": SourceNetworkText,
	"approval.Request.Summary":             SourceProjectText,
	"approval.Request.Reason":              SourceProjectText,
	"service.TextResult.Text":              SourceNetworkText,
	"service.ModelView.Name":               SourceNetworkText,
	"service.LibraryEntry.Description":     SourceLibraryText,
	"service.LibraryEntry.Tags":            SourceLibraryText,
	"service.LibraryEntry.Error":           SourceLibraryText,
	"service.LibraryHit.Snippet":           SourceLibraryText,
	"service.MemoryGetResult.Body":         SourceLibraryText,
	"service.PromptGetResult.Body":         SourceLibraryText,
	"service.PromptGetResult.Defaults":     SourceLibraryText,
	"service.PromptRenderResult.Rendered":  SourceLibraryText,
	"service.SkillGetResult.License":       SourceLibraryText,
	"service.SkillGetResult.Compatibility": SourceLibraryText,
	"service.SkillGetResult.AllowedTools":  SourceLibraryText,
	"service.SkillGetResult.Source":        SourceLibraryText,
	"service.SkillGetResult.Body":          SourceLibraryText,
	"service.SkillFileView.Text":           SourceLibraryText,
	"service.LibraryReindexResult.Ignored": SourceLibraryText,

	// publishing (SDD-020): the channel, the video the network named and what a person would approve
	"service.PublishResult.RemoteID":        SourceNetworkText,
	"service.PublishResult.URL":             SourceNetworkText,
	"service.SocialAccount.Label":           SourceProjectText,
	"service.SocialAccount.AccountTitle":    SourceNetworkText,
	"service.PublishValidateResult.Summary": SourceProjectText,

	// the sign-in to an AI provider (SDD-016 section 12): the name a person gave the connection, and who the
	// provider says the account is
	"service.ProviderSignIn.Label":   SourceProjectText,
	"service.ProviderSignIn.Account": SourceNetworkText,

	// the asset library (SDD-011 section 14): the text a person or an agent wrote about a file
	"assets.LibraryAsset.Description": SourceMetadata,
	"assets.Use.Name":                 SourceProjectText,
	"asset.Origin.Source":             SourceMetadata,
	"asset.Origin.Prompt":             SourceMetadata,
	"asset.Origin.RightsNote":         SourceMetadata,

	// the hand-off from an external motion engine (SDD-011 section 15): the words of the captions in the range
	"service.MotionCaption.Text":          SourceProjectText,
	"service.MotionCheckResult.AssetName": SourceAssetName,
	"service.RenderOffer.Name":            SourceAssetName,
}
