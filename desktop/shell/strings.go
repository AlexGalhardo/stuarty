package shell

import (
	"os"
	"strings"
)

// The few texts of the native menu and dialogs, in the languages of the editor
// (en and pt-BR). The shell does not know the language chosen inside the page,
// so it follows the system language and falls back to English.
const (
	TextFile          = "file"
	TextOpenWorkspace = "open_workspace"
	TextQuit          = "quit"
	TextView          = "view"
	TextReload        = "reload"
	TextFullScreen    = "full_screen"
	TextHelp          = "help"
	TextCheckUpdates  = "check_updates"
	TextCheckAtStart  = "check_at_start"
	TextAbout         = "about"
	TextAboutTitle    = "about_title"
	TextInstall       = "install"
	TextNotNow        = "not_now"
	TextCannotStart   = "cannot_start"
	TextServerStopped = "server_stopped"
	TextCannotOpen    = "cannot_open"
	TextCannotSave    = "cannot_save"
	TextConsentTitle  = "consent_title"
	TextConsentBody   = "consent_body"
	TextYes           = "yes"
	TextNo            = "no"
	TextDownloading   = "downloading"
	TextUndone        = "undone"
	TextUpToDate      = "up_to_date"
	TextOffer         = "offer"
	TextNoFeed        = "no_feed"
	TextCheckFailed   = "check_failed"
	TextInstallFailed = "install_failed"
)

var catalogues = map[string]map[string]string{
	"en": {
		TextFile: "File", TextOpenWorkspace: "Open workspace folder", TextQuit: "Quit",
		TextView: "View", TextReload: "Reload", TextFullScreen: "Toggle full screen",
		TextHelp: "Help", TextCheckUpdates: "Check for updates",
		TextCheckAtStart: "Check for updates automatically", TextAbout: "About",
		TextAboutTitle: "About Vukem Video", TextInstall: "Install and restart", TextNotNow: "Not now",
		TextCannotStart: "Vukem Video cannot start", TextServerStopped: "The local server stopped unexpectedly.",
		TextCannotOpen: "Cannot open the folder: ", TextCannotSave: "Cannot save the setting: ",
		TextConsentTitle: "Check for updates automatically?",
		TextConsentBody: "Vukem Video can look for a new version when it starts and once a day. " +
			"It connects only to the update server, only for this check, and sends nothing about you. " +
			"You can change this in Help.",
		TextYes: "Yes", TextNo: "No", TextDownloading: "Downloading Vukem Video ",
		TextUndone:   "Vukem Video could not start the new version, so the previous version was restored. Version ",
		TextUpToDate: "Vukem Video %s is up to date.",
		TextOffer:    "Version %s is available (this is %s). Download and install it?",
		TextNoFeed: "This build has no update feed or signing key, so it cannot check for updates. " +
			"Download new versions from %s",
		TextCheckFailed:      "Could not check for updates: ",
		TextInstallFailed:    "The update was not installed and this version is unchanged: ",
		TextMenuEdit:         "Edit",
		TextMenuClip:         "Clip",
		TextMenuTimeline:     "Timeline",
		TextMenuOpenProject:  "Open project",
		TextMenuImport:       "Import files",
		TextMenuExport:       "Export",
		TextMenuUndo:         "Undo",
		TextMenuRedo:         "Redo",
		TextMenuCut:          "Cut",
		TextMenuCopy:         "Copy",
		TextMenuPaste:        "Paste",
		TextMenuSelectAll:    "Select all",
		TextMenuDelete:       "Delete",
		TextMenuDuplicate:    "Duplicate",
		TextMenuSplit:        "Split",
		TextMenuTrimStart:    "Trim start",
		TextMenuTrimEnd:      "Trim end",
		TextMenuSpeed:        "Speed",
		TextMenuRename:       "Rename",
		TextMenuFreezeFrame:  "Freeze frame",
		TextMenuToggle:       "Turn on or off",
		TextMenuAddVideo:     "Add video track",
		TextMenuAddAudio:     "Add audio track",
		TextMenuSnap:         "Snap",
		TextMenuMagnet:       "Magnet",
		TextMenuMarker:       "Marker at the playhead",
		TextMenuAddCaption:   "Add caption",
		TextMenuAddText:      "Add text",
		TextMenuPanelMedia:   "Media panel",
		TextMenuPanelInspect: "Inspector panel",
		TextMenuPanelHistory: "History panel",
		TextMenuShortcuts:    "Keyboard shortcuts",
	},
	"pt-BR": {
		TextFile:          "Arquivo",
		TextOpenWorkspace: "Abrir pasta do workspace",
		TextQuit:          "Sair",
		TextView:          "Exibir",
		TextReload:        "Recarregar",
		TextFullScreen:    "Tela cheia",
		TextHelp:          "Ajuda",
		TextCheckUpdates:  "Verificar atualizações",
		TextCheckAtStart:  "Verificar atualizações automaticamente",
		TextAbout:         "Sobre",
		TextAboutTitle:    "Sobre o Vukem Video",
		TextInstall:       "Instalar e reiniciar",
		TextNotNow:        "Agora não",
		TextCannotStart:   "O Vukem Video não consegue iniciar",
		TextServerStopped: "O servidor local parou de repente.",
		TextCannotOpen:    "Não foi possível abrir a pasta: ",
		TextCannotSave:    "Não foi possível salvar a configuração: ",
		TextConsentTitle:  "Verificar atualizações automaticamente?",
		TextConsentBody: "O Vukem Video pode procurar uma versão nova ao iniciar e uma vez por dia. " +
			"Ele se conecta só ao servidor de atualizações, só para essa verificação, e não envia nada sobre você. " +
			"Você pode mudar isso em Ajuda.",
		TextYes:         "Sim",
		TextNo:          "Não",
		TextDownloading: "Baixando o Vukem Video ",
		TextUndone:      "O Vukem Video não conseguiu iniciar a versão nova e restaurou a anterior. Versão ",
		TextUpToDate:    "O Vukem Video %s está atualizado.",
		TextOffer:       "A versão %s está disponível (esta é a %s). Baixar e instalar?",
		TextNoFeed: "Esta versão não tem endereço de atualizações nem chave de assinatura, então não consegue verificar atualizações. " +
			"Baixe as versões novas em %s",
		TextCheckFailed:      "Não foi possível verificar atualizações: ",
		TextInstallFailed:    "A atualização não foi instalada e esta versão continua como estava: ",
		TextMenuEdit:         "Editar",
		TextMenuClip:         "Clipe",
		TextMenuTimeline:     "Linha do tempo",
		TextMenuOpenProject:  "Abrir projeto",
		TextMenuImport:       "Importar arquivos",
		TextMenuExport:       "Exportar",
		TextMenuUndo:         "Desfazer",
		TextMenuRedo:         "Refazer",
		TextMenuCut:          "Recortar",
		TextMenuCopy:         "Copiar",
		TextMenuPaste:        "Colar",
		TextMenuSelectAll:    "Selecionar tudo",
		TextMenuDelete:       "Excluir",
		TextMenuDuplicate:    "Duplicar",
		TextMenuSplit:        "Dividir",
		TextMenuTrimStart:    "Cortar o início",
		TextMenuTrimEnd:      "Cortar o fim",
		TextMenuSpeed:        "Velocidade",
		TextMenuRename:       "Renomear",
		TextMenuFreezeFrame:  "Congelar quadro",
		TextMenuToggle:       "Ativar ou desativar",
		TextMenuAddVideo:     "Adicionar faixa de vídeo",
		TextMenuAddAudio:     "Adicionar faixa de áudio",
		TextMenuSnap:         "Encaixar",
		TextMenuMagnet:       "Ímã",
		TextMenuMarker:       "Marcador no cabeçote",
		TextMenuAddCaption:   "Adicionar legenda",
		TextMenuAddText:      "Adicionar texto",
		TextMenuPanelMedia:   "Painel de mídia",
		TextMenuPanelInspect: "Painel do inspetor",
		TextMenuPanelHistory: "Painel de histórico",
		TextMenuShortcuts:    "Atalhos de teclado",
	},
}

// Language maps a system locale name ("pt_BR.UTF-8", "pt-PT", "en-US") to a
// catalogue: every Portuguese variant is pt-BR, anything else English.
func Language(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "pt") {
		return "pt-BR"
	}
	return "en"
}

// Text is the string for key in the language; a missing one falls back to English.
func Text(language, key string) string {
	if s, ok := catalogues[language][key]; ok {
		return s
	}
	return catalogues["en"][key]
}

// SystemLanguage is the catalogue for the language of the system.
func SystemLanguage() string { return Language(systemLocale(os.Getenv)) }
