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
)

var catalogues = map[string]map[string]string{
	"en": {
		TextFile: "File", TextOpenWorkspace: "Open workspace folder", TextQuit: "Quit",
		TextView: "View", TextReload: "Reload", TextFullScreen: "Toggle full screen",
		TextHelp: "Help", TextCheckUpdates: "Check for updates",
		TextCheckAtStart: "Check for updates at start-up", TextAbout: "About",
		TextAboutTitle: "About Vukem Video", TextInstall: "Install and restart", TextNotNow: "Not now",
		TextCannotStart: "Vukem Video cannot start", TextServerStopped: "The local server stopped unexpectedly.",
		TextCannotOpen: "Cannot open the folder: ", TextCannotSave: "Cannot save the setting: ",
	},
	"pt-BR": {
		TextFile: "Arquivo", TextOpenWorkspace: "Abrir pasta do workspace", TextQuit: "Sair",
		TextView: "Exibir", TextReload: "Recarregar", TextFullScreen: "Tela cheia",
		TextHelp: "Ajuda", TextCheckUpdates: "Verificar atualizações",
		TextCheckAtStart: "Verificar atualizações ao iniciar", TextAbout: "Sobre",
		TextAboutTitle: "Sobre o Vukem Video", TextInstall: "Instalar e reiniciar", TextNotNow: "Agora não",
		TextCannotStart: "O Vukem Video não consegue iniciar", TextServerStopped: "O servidor local parou de repente.",
		TextCannotOpen: "Não foi possível abrir a pasta: ", TextCannotSave: "Não foi possível salvar a configuração: ",
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
