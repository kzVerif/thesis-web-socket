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
	"time"
	"ws-rat/internal/downloadtoken"
)

type downloadResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *downloadResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *downloadResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

// Unwrap keeps http.ResponseController features available through the logger.
func (w *downloadResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) HandleFileDownload(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	lw := &downloadResponseWriter{ResponseWriter: w}
	defer func() {
		status := lw.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf("download GET complete status=%d bytes=%d duration=%s path=%q agent=%q",
			status, lw.bytes, time.Since(started).Round(time.Millisecond), r.URL.Path, r.Header.Get("X-Agent-ID"))
	}()
	log.Printf("download GET start method=%s host=%q path=%q remote=%q agent=%q",
		r.Method, r.Host, r.URL.Path, r.RemoteAddr, r.Header.Get("X-Agent-ID"))

	if r.Method != http.MethodGet {
		lw.Header().Set("Allow", "GET")
		http.Error(lw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/files/download/")
	agent := r.Header.Get("X-Agent-ID")
	if raw == "" || agent == "" {
		log.Printf("download GET rejected reason=missing_token_or_agent token_length=%d agent=%q", len(raw), agent)
		http.Error(lw, "invalid download grant", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), databaseTimeout)
	defer cancel()
	f, err := s.distributionRepo.ResolveGrant(ctx, downloadtoken.Hash(raw), agent)
	if err != nil {
		log.Printf("download GET rejected reason=invalid_or_expired_grant err=%q agent=%q", err, agent)
		http.Error(lw, "invalid or expired download grant", http.StatusUnauthorized)
		return
	}
	log.Printf("download GET grant_resolved file_id=%q filename=%q storage_path=%q expected_bytes=%d agent=%q",
		f.ID, f.Filename, f.StoragePath, f.Size, agent)
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
		log.Printf("download GET file_open_failed storage_path=%q err=%q agent=%q", f.StoragePath, err, agent)
		if errors.Is(err, os.ErrNotExist) {
			http.Error(lw, "file unavailable", http.StatusNotFound)
		} else {
			http.Error(lw, "file unavailable", http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()
	st, err := file.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() != f.Size {
		actualBytes := int64(-1)
		regular := false
		if err == nil {
			actualBytes = st.Size()
			regular = st.Mode().IsRegular()
		}
		log.Printf("download GET metadata_mismatch storage_path=%q stat_error=%v actual_bytes=%d expected_bytes=%d regular=%t agent=%q",
			f.StoragePath, err, actualBytes, f.Size, regular, agent)
		http.Error(lw, "file metadata mismatch", http.StatusConflict)
		return
	}
	name := filepath.Base(strings.ReplaceAll(f.Filename, "\"", ""))
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	lw.Header().Set("Content-Type", contentType)
	lw.Header().Set("Content-Length", fmt.Sprint(f.Size))
	lw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	lw.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(lw, r, name, st.ModTime(), file)
}
