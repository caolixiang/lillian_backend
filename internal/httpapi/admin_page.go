package httpapi

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
)

//go:embed admin_dist
var adminDistFS embed.FS

func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	index, err := fs.ReadFile(adminDistFS, "admin_dist/index.html")
	if err != nil {
		_, _ = w.Write([]byte(adminFallbackHTML))
		return
	}
	_, _ = w.Write(index)
}

func (s *Server) handleAdminAsset(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(adminDistFS, "admin_dist")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.StripPrefix("/admin/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	icon, err := fs.ReadFile(adminDistFS, "admin_dist/lillian-icon.svg")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if err == nil {
		_, _ = w.Write(icon)
		return
	}
	_, _ = w.Write([]byte(lillianIconSVG))
}

const adminFallbackHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>莉莉安的后台 | Lillian's Canvas Admin</title>
  <link rel="icon" href="/lillian-icon.svg" type="image/svg+xml">
</head>
<body>
  <main style="font-family: system-ui, sans-serif; max-width: 720px; margin: 80px auto; line-height: 1.6;">
    <h1>莉莉安的后台</h1>
    <p>Admin frontend is not built. Run <code>npm --prefix web/admin run build</code> before starting the backend.</p>
  </main>
</body>
</html>`

const lillianIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#f8f5ff"/><path d="M18 44c8-2 13-8 14-20 1 12 6 18 14 20-7 5-21 5-28 0Z" fill="#8f7af4"/><path d="M32 12c4 6 5 13 0 21-5-8-4-15 0-21Z" fill="#5849bf"/><path d="M21 24c7 0 11 4 11 11-8-1-12-5-11-11Z" fill="#d9b7f5"/><path d="M43 24c-7 0-11 4-11 11 8-1 12-5 11-11Z" fill="#b7d7f5"/><circle cx="32" cy="38" r="4" fill="#26233f"/></svg>`
