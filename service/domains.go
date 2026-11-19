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
	"ProjectBundleExport": command.DomainProject, "ProjectBundleImport": command.DomainProject,

	"Edit": command.DomainVideo, "HistoryList": command.DomainVideo, "Undo": command.DomainVideo,
	"Redo": command.DomainVideo, "Lint": command.DomainVideo, "ViewFrame": command.DomainVideo,
	"ViewSheet": command.DomainVideo, "JobStart": command.DomainVideo, "JobGet": command.DomainVideo,
	"JobList": command.DomainVideo, "JobCancel": command.DomainVideo, "ExportFile": command.DomainVideo,
	"TemplateList": command.DomainVideo, "TemplateSave": command.DomainVideo,
	"TemplateInstantiate": command.DomainVideo, "TemplateExport": command.DomainVideo,
	"TemplateImport": command.DomainVideo, "SnapPoints": command.DomainVideo, "PlanWindow": command.DomainVideo,
	"OpenFile": command.DomainVideo, "ProjectOtioExport": command.DomainVideo, "ProjectOtioImport": command.DomainVideo,

	"ImageCreate": command.DomainImage, "ImageList": command.DomainImage, "ImageDescribe": command.DomainImage,
	"ImageEdit": command.DomainImage, "ImageUndo": command.DomainImage, "ImageRedo": command.DomainImage,
	"ImageExport": command.DomainImage,

	"MediaImport": command.DomainAsset, "MediaInspect": command.DomainAsset, "MediaList": command.DomainAsset,
	"MediaTrash": command.DomainAsset, "MediaRestore": command.DomainAsset, "AssetList": command.DomainAsset,
	"FontImport": command.DomainAsset, "FontList": command.DomainAsset,

	"MemoryList": command.DomainMemory, "MemoryGet": command.DomainMemory, "MemoryWrite": command.DomainMemory,
	"MemoryRemove": command.DomainMemory, "SkillList": command.DomainMemory, "SkillGet": command.DomainMemory,
	"SkillWrite": command.DomainMemory, "SkillImport": command.DomainMemory, "SkillExport": command.DomainMemory,
	"SkillMarkReviewed": command.DomainMemory, "SkillLint": command.DomainMemory, "PromptList": command.DomainMemory,
	"PromptGet": command.DomainMemory, "PromptWrite": command.DomainMemory, "PromptRender": command.DomainMemory,
	"LibrarySearch": command.DomainMemory, "LibraryReindex": command.DomainMemory,

	"CommandHelp": command.DomainWorkspace, "ConnectionList": command.DomainWorkspace,
	"ConnectionGet": command.DomainWorkspace, "ConnectionCreate": command.DomainWorkspace,
	"ConnectionRemove": command.DomainWorkspace, "ConnectionTest": command.DomainWorkspace,
	"BrandKitGet": command.DomainWorkspace, "BrandKitSet": command.DomainWorkspace,
	"Doctor": command.DomainWorkspace, "AuditList": command.DomainWorkspace, "TrashEmpty": command.DomainWorkspace,
	"ToolsInstall": command.DomainWorkspace, "ToolsStatus": command.DomainWorkspace,
	"KeyCreate": command.DomainWorkspace, "KeyList": command.DomainWorkspace, "KeyRevoke": command.DomainWorkspace,
}

// OperationDomain is the domain of a Service method.
func OperationDomain(method string) (string, bool) {
	d, ok := OperationDomains[method]

	return d, ok
}
