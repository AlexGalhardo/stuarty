package main

import "github.com/AlexGalhardo/vukem-video/desktop/shell"

// tr is the native text for key in the language of the system.
func tr(key string) string { return shell.Text(shell.SystemLanguage(), key) }
