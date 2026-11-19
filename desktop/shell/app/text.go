package main

import "github.com/AlexGalhardo/stuarty/desktop/shell"

// tr is the native text for key in the language of the system.
func tr(key string) string { return shell.Text(shell.SystemLanguage(), key) }
