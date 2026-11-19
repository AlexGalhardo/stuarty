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
	"service.LibraryAsset.Name":                SourceAssetName,
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
}
