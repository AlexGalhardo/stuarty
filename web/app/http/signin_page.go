package httpapi

import (
	"net/http"
)

// signInHint says how to get in when a request carries no credentials in local
// mode. It names the command, never a token or a launch code.
const signInHint = "send the local token as Authorization: Bearer <token>, or run `stuarty open` to open the editor in a browser"

// docsSignInURL is the documentation page that explains the way in.
const docsSignInURL = "https://video.vukem.dev/docs/self-hosting/binary"

const signInPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>401 Sign-in needed</title>
</head>
<body>
<main>
<h1>401 Sign-in needed</h1>
<p>This browser has no session with the local Stuarty server.</p>
<p>Run <code>stuarty open</code> in a terminal: it opens the editor here with a one-time link that works for two minutes. The desktop application does this in its own window.</p>
<p><a href="` + docsSignInURL + `" rel="noopener noreferrer">How to open the editor</a></p>
</main>
</body>
</html>
`

// writeSignInPage answers a page request with no session in local mode. The page
// is static: it reveals neither the token nor a launch code (SDD-007 section 3.4).
func writeSignInPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(signInPage))
}
