package activation

import (
	"html/template"
	"io"
)

var activationPage = template.Must(template.New("activation").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="referrer" content="no-referrer">
<title>Activate Stickguy</title>
<main><h1>Continue to Stickguy</h1><p>This sends a short-lived, single-use ticket directly to your hosted Stickguy session. The ticket is not placed in the URL or browser storage.</p>
<form method="post" action="{{.Action}}"><input type="hidden" name="ticket" value="{{.Ticket}}"><button type="submit">Open secure dashboard</button></form></main></html>`))

func writePage(w io.Writer, action, ticket string) error {
	return activationPage.Execute(w, struct{ Action, Ticket string }{action, ticket})
}
