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

	// the asset library (SDD-011 section 14): the text a person or an agent wrote about a file
	"assets.LibraryAsset.Description": SourceMetadata,
	"assets.Use.Name":                 SourceProjectText,
	"asset.Origin.Source":             SourceMetadata,
	"asset.Origin.Prompt":             SourceMetadata,
	"asset.Origin.RightsNote":         SourceMetadata,
}
