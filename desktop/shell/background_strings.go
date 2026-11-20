package shell

import (
	"fmt"
	"maps"
)

// The texts of the tray and of the quit warning of the background mode (SDD-012 section 21.3 item 7).
const (
	TextTrayOpen       = "tray_open"
	TextTrayPause      = "tray_pause"
	TextTrayTooltip    = "tray_tooltip"
	TextQuitTitle      = "quit_title"
	TextQuitWaitingOne = "quit_waiting_one"
	TextQuitWaitingMan = "quit_waiting_many"
	TextQuitConfirm    = "quit_confirm"
	TextKeepRunning    = "keep_running"
	TextCannotPause    = "cannot_pause"
)

func init() {
	for lang, texts := range map[string]map[string]string{
		"en": {
			TextTrayOpen:       "Open Stuarty",
			TextTrayPause:      "Pause publishing",
			TextTrayTooltip:    "Stuarty is running in the background",
			TextQuitTitle:      "Quit Stuarty?",
			TextQuitWaitingOne: "One scheduled post will wait until Stuarty runs again.",
			TextQuitWaitingMan: "%d scheduled posts will wait until Stuarty runs again.",
			TextQuitConfirm:    "Quit",
			TextKeepRunning:    "Keep running",
			TextCannotPause:    "Could not change publishing: ",
		},
		"pt-BR": {
			TextTrayOpen:       "Abrir o Stuarty",
			TextTrayPause:      "Pausar as publicações",
			TextTrayTooltip:    "O Stuarty está rodando em segundo plano",
			TextQuitTitle:      "Sair do Stuarty?",
			TextQuitWaitingOne: "Uma publicação agendada vai esperar até o Stuarty rodar de novo.",
			TextQuitWaitingMan: "%d publicações agendadas vão esperar até o Stuarty rodar de novo.",
			TextQuitConfirm:    "Sair",
			TextKeepRunning:    "Continuar rodando",
			TextCannotPause:    "Não foi possível mudar as publicações: ",
		},
	} {
		maps.Copy(catalogues[lang], texts)
	}
}

// QuitWaitingMessage is what Quit from the tray says about the scheduled posts that will wait.
func QuitWaitingMessage(language string, count int) string {
	if count == 1 {
		return Text(language, TextQuitWaitingOne)
	}

	return fmt.Sprintf(Text(language, TextQuitWaitingMan), count)
}
