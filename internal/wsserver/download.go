package wsserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"ws-rat/internal/downloadtoken"
)

func (s *Server) HandleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/files/download/")
	agent := r.Header.Get("X-Agent-ID")
	if raw == "" || agent == "" {
		http.Error(w, "invalid download grant", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), databaseTimeout)
	defer cancel()
	f, err := s.distributionRepo.ResolveGrant(ctx, downloadtoken.Hash(raw), agent)
	log.Printf(
		"file download: method=%s path=%s storage_path=%q agent=%q",
		r.Method,
		r.URL.Path,
		f.StoragePath,
		agent,
	)
	if err != nil {
		http.Error(w, "invalid or expired download grant", http.StatusUnauthorized)
		return
	}
	// root, err := filepath.Abs(s.storageRoot)
	// log.Println("root:", root)
	// if err != nil {
	// 	http.Error(w, "file unavailable", http.StatusInternalServerError)
	// 	return
	// }
	// path, err := filepath.Abs(filepath.Join(root, f.StoragePath))
	// if err != nil {
	// 	http.Error(w, "file unavailable", http.StatusNotFound)
	// 	return
	// }
	// rel, err := filepath.Rel(root, path)
	// if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
	// 	http.Error(w, "file unavailable", http.StatusForbidden)
	// 	return
	// }
	file, err := os.Open(f.StoragePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "file unavailable", http.StatusNotFound)
		} else {
			http.Error(w, "file unavailable", http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()
	st, err := file.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() != f.Size {
		http.Error(w, "file metadata mismatch", http.StatusConflict)
		return
	}
	name := filepath.Base(strings.ReplaceAll(f.Filename, "\"", ""))
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprint(f.Size))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, st.ModTime(), file)
}
