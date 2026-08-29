package activation

import (
	"crypto/sha256"
	"encoding/base64"
	"html/template"
	"io"
)

// The handoff page exists to move a single-use ticket to the API as a top-level
// POST, so that it never enters a URL, browser history, a Referer header, or
// browser storage (ADR-024). That is the whole of its job.
//
// It used to ask for a click to do it. The page is unavoidably a stop on the
// way to somewhere else, and it opens in whichever browser the member has - so
// a button here meant the app appeared to do nothing while a tab waited,
// unlabelled, among however many others were already open, for a press that
// carried no decision. Nothing is being consented to on this page: the member
// already asked for the Project, and the disclosure of what a Project shares
// lives in the app before enrollment and on the workroom's activation screen.
//
// So it submits itself. The button remains for the case where the script does
// not run, which is the only case where a person still needs to act.
const activationScript = `document.forms[0].submit()`

// The page can still be seen - for the frame before it posts, and for as long
// as it takes a person to press the button when the script cannot run - so it
// is styled rather than left as raw user-agent defaults.
const activationStyle = `body{margin:0;min-height:100vh;display:grid;place-items:center;font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#0e0e0e;background:#fff}main{max-width:34rem;padding:2rem;text-align:center}h1{font-size:1.1rem;font-weight:600;margin:0 0 .5rem}p{color:#5e5e5a;margin:0}button{margin-top:1.25rem;padding:.55rem 1rem;font:inherit;font-weight:600;color:#fff;background:#0e0e0e;border:0;border-radius:.5rem;cursor:pointer}@media(prefers-color-scheme:dark){body{color:#f5f5f5;background:#0a0a0a}p{color:#a3a39f}button{color:#0a0a0a;background:#fff}}`

// scriptHash pins the CSP to this exact script rather than relaxing it to
// 'unsafe-inline'; any edit to activationScript changes the hash and the
// browser refuses to run it, which is the intended failure mode.
var scriptHash = cspHash(activationScript)

// styleHash keeps style-src off 'unsafe-inline' for the same reason scriptHash
// keeps script-src off it: one known payload runs, and nothing injected does.
var styleHash = cspHash(activationStyle)

func cspHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

var activationPage = template.Must(template.New("activation").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="referrer" content="no-referrer">
<title>Activate Stickguy</title>
<style>{{.Style}}</style>
<main><h1>Opening your Stickguy workroom…</h1><p>Sending a short-lived, single-use ticket to your hosted session. It is not placed in the URL or browser storage.</p>
<form method="post" action="{{.Action}}"><input type="hidden" name="ticket" value="{{.Ticket}}"><button type="submit">Continue</button></form></main>
<script>{{.Script}}</script></html>`))

func writePage(w io.Writer, action, ticket string) error {
	return activationPage.Execute(w, struct {
		Action, Ticket string
		Script         template.JS
		Style          template.CSS
	}{action, ticket, template.JS(activationScript), template.CSS(activationStyle)})
}
