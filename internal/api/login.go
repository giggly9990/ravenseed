package api

import (
	"html/template"
	"net/http"
)

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Sign in | RavenSeed</title>
<style>:root{color-scheme:dark;font-family:system-ui,sans-serif;background:#111416;color:#f1f1f1}*{box-sizing:border-box}body{min-height:100vh;margin:0;display:grid;place-items:center;padding:24px}main{width:100%;max-width:420px;border:1px solid #393332;border-radius:16px;background:#1b1c1e;padding:32px}h1{margin:12px 0 4px;font-size:28px}p{color:#b7b4b3;line-height:1.5}label{display:block;margin:20px 0 8px;font-size:14px}input,button{font:inherit;width:100%;border-radius:8px;padding:12px}input{background:#111416;color:#fff;border:1px solid #555}input:focus{outline:2px solid #e14d4d;outline-offset:2px}button{margin-top:24px;border:0;background:#bd3535;color:white;cursor:pointer;font-weight:600}button:hover{background:#d74242}.error{color:#ffb0a6;border-left:3px solid #bd3535;padding-left:12px}svg{width:54px;height:54px;fill:white}.hint{font-size:13px}</style></head>
<body><main><svg aria-hidden="true" viewBox="0 0 128 128"><path d="M112 16.43 106.86 11.29 93.14 7 84.57 8.71 78.57 12.14 58 35.29 27.14 74.71 30.57 75.57 37.43 73.86 38.29 74.71 4 109 23.71 109 39.14 95.29 52 98.71 53.71 105.57 58.86 110.71 59.71 115 57.14 117.57 62.29 117.57 66.57 120.14 69.14 120.14 70 118.43 71.71 119.29 70.86 116.71 64 115 62.29 112.43 65.71 99.57 68.29 97 75.14 95.29 88 87.57 98.29 75.57 101.71 65.29 100.86 46.43 95.71 49.86 90.57 50.71 81.14 48.14 73.43 39.57 73.43 31 79.43 24.14 79.43 38.71 85.43 43.86 95.71 43.86 101.71 38.71 103.43 33.57 100 36.14 94 35.29 89.71 30.14 90.57 25 95.71 19.86 106.86 18.14 110.29 21.57 105.14 21.57 108.57 31.86 105.14 42.14 114.57 47.29 120.57 55 123.14 48.14 123.14 42.14 120.57 35.29 114.57 28.43 115.43 25Z"/></svg>
<h1>RavenSeed</h1><p>Sign in to your workspace.</p>
{{if .Message}}<p class="error" role="alert">{{.Message}}</p>{{end}}
{{if .Enabled}}<form method="post" action="/login"><input type="hidden" name="next" value="{{.Next}}"><label for="username">Username</label><input id="username" name="username" autocomplete="username" maxlength="128" required autofocus><label for="password">Password</label><input id="password" name="password" type="password" autocomplete="current-password" maxlength="1024" required><button type="submit">Sign in</button></form>{{end}}
<p class="hint">Your indexers. One view.</p></main></body></html>`))

func (a *API) loginPage(w http.ResponseWriter, r *http.Request, status int, message, next string) {
	enabled := a.Service.Config.WebPasswordHash != ""
	if !enabled && message == "" {
		message = "Browser login is not configured. Set WEB_USERNAME and a web password on the server."
	}
	if a.Service.Config.WebCookieSecure && !a.secureRequest(r) {
		enabled = false
		if message == "" {
			message = "HTTPS is required. For localhost HTTP, configure WEB_COOKIE_SECURE=false."
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.WriteHeader(status)
	_ = loginTemplate.Execute(w, struct {
		Message, Next string
		Enabled       bool
	}{message, next, enabled})
}
