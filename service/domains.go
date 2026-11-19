package service

import "github.com/AlexGalhardo/stuarty/core/command"

// OperationDomains is the domain of every method of Service (SDD-000 section 1.1, ADR-0043): the registry of the
// operations that are not edit commands, whose domains the command registries carry themselves. A method with no
// entry fails TestOperationDomains, so a later epic cannot add an operation that no domain owns.
var OperationDomains = map[string]string{
	"ProjectList": command.DomainProject, "ProjectCreate": command.DomainProject, "ProjectGet": command.DomainProject,
	"ProjectTrash": command.DomainProject, "ProjectRestore": command.DomainProject,
	"ProjectDescribe": command.DomainProject, "ProjectDuplicate": command.DomainProject,
	"ProjectItemAdd": command.DomainProject, "ProjectItemMove": command.DomainProject,
	"ProjectItemRename": command.DomainProject, "ProjectItemTrash": command.DomainProject,
	"ProjectItemRestore": command.DomainProject, "BriefGet": command.DomainProject, "BriefSet": command.DomainProject,
	"BriefingQuestions": command.DomainProject, "BriefingAnswer": command.DomainProject,
	"ProjectBundleExport": command.DomainProject, "ProjectBundleImport": command.DomainProject,

	"Edit": command.DomainVideo, "HistoryList": command.DomainVideo, "Undo": command.DomainVideo,
	"Redo": command.DomainVideo, "Lint": command.DomainVideo, "ViewFrame": command.DomainVideo,
	"ViewSheet": command.DomainVideo, "JobStart": command.DomainVideo, "JobGet": command.DomainVideo,
	"JobList": command.DomainVideo, "JobCancel": command.DomainVideo, "ExportFile": command.DomainVideo,
	"TemplateList": command.DomainVideo, "TemplateSave": command.DomainVideo,
	"TemplateInstantiate": command.DomainVideo, "TemplateExport": command.DomainVideo,
	"TemplateImport": command.DomainVideo, "SnapPoints": command.DomainVideo, "PlanWindow": command.DomainVideo,
	"OpenFile": command.DomainVideo, "ProjectOtioExport": command.DomainVideo, "ProjectOtioImport": command.DomainVideo,
	"ExportPresetList": command.DomainVideo, "ExportCheck": command.DomainVideo, "ExportBatch": command.DomainVideo,

	"ImageCreate": command.DomainImage, "ImageList": command.DomainImage, "ImageDescribe": command.DomainImage,
	"ImageEdit": command.DomainImage, "ImageUndo": command.DomainImage, "ImageRedo": command.DomainImage,
	"ImageExport": command.DomainImage, "ImageFrame": command.DomainImage, "ImageHistory": command.DomainImage,
	"ImageFile": command.DomainImage,

	"MediaImport": command.DomainAsset, "MediaInspect": command.DomainAsset, "MediaList": command.DomainAsset,
	"MediaTrash": command.DomainAsset, "MediaRestore": command.DomainAsset, "AssetList": command.DomainAsset,
	"FontImport": command.DomainAsset, "FontList": command.DomainAsset,
	"AssetGet": command.DomainAsset, "AssetUses": command.DomainAsset, "AssetSetInfo": command.DomainAsset,
	"AssetTag": command.DomainAsset,

	"MemoryList": command.DomainMemory, "MemoryGet": command.DomainMemory, "MemoryWrite": command.DomainMemory,
	"MemoryRemove": command.DomainMemory, "SkillList": command.DomainMemory, "SkillGet": command.DomainMemory,
	"SkillWrite": command.DomainMemory, "SkillImport": command.DomainMemory, "SkillExport": command.DomainMemory,
	"SkillMarkReviewed": command.DomainMemory, "SkillLint": command.DomainMemory, "PromptList": command.DomainMemory,
	"PromptGet": command.DomainMemory, "PromptWrite": command.DomainMemory, "PromptRender": command.DomainMemory,
	"LibrarySearch": command.DomainMemory, "LibraryReindex": command.DomainMemory,

	// The approval queue is the gate of a publication and of a costly generation (ADR-0032); the cost summary and
	// its ceilings are the spend on providers.
	"ApprovalList": command.DomainPublish, "ApprovalGet": command.DomainPublish,
	"ApprovalDecide": command.DomainPublish, "ApprovalRuleList": command.DomainPublish,
	"ApprovalRuleSet": command.DomainPublish, "ApprovalRuleRemove": command.DomainPublish,
	"CostSummary": command.DomainProvider, "CostCeilingSet": command.DomainProvider,

	"CommandHelp": command.DomainWorkspace, "ConnectionList": command.DomainWorkspace,
	"ConnectionGet": command.DomainWorkspace, "ConnectionCreate": command.DomainWorkspace,
	"ConnectionRemove": command.DomainWorkspace, "ConnectionTest": command.DomainWorkspace,
	"BrandKitGet": command.DomainWorkspace, "BrandKitSet": command.DomainWorkspace,
	"Doctor": command.DomainWorkspace, "AuditList": command.DomainWorkspace, "TrashEmpty": command.DomainWorkspace,
	"ToolsInstall": command.DomainWorkspace, "ToolsStatus": command.DomainWorkspace,
	"KeyCreate": command.DomainWorkspace, "KeyList": command.DomainWorkspace, "KeyRevoke": command.DomainWorkspace,

	// The agent the user hosted in the chat panel (SDD-017).
	"AgentProfileList": command.DomainWorkspace, "AgentProfileSet": command.DomainWorkspace,
	"AgentProfileRemove": command.DomainWorkspace, "ChatSessionStart": command.DomainWorkspace,
	"ChatSend": command.DomainWorkspace, "ChatCancel": command.DomainWorkspace,
	"ChatAnswerPermission": command.DomainWorkspace, "ChatSessionDelete": command.DomainWorkspace,
	"ChatSessionList": command.DomainWorkspace, "ChatSessionGet": command.DomainWorkspace,
	"ChatTranscriptGet": command.DomainWorkspace,
}

// OperationDomain is the domain of a Service method.
func OperationDomain(method string) (string, bool) {
	d, ok := OperationDomains[method]

	return d, ok
}
