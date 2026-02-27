package api

import (
	"encoding/json"
	"net/http"

	apperrors "kopelan/mingyue-go/internal/errors"
	fileService "kopelan/mingyue-go/internal/service/file"
)

// fileListHandler handles GET /api/v1/files
// Query parameter: path (required)
func fileListHandler(mgr *fileService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "path query parameter is required"))
			return
		}

		entries, err := mgr.List(r.Context(), path)
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"path":    path,
			"entries": entries,
		})
	}
}

// fileStatHandler handles GET /api/v1/files/stat
// Query parameter: path (required)
func fileStatHandler(mgr *fileService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "path query parameter is required"))
			return
		}

		fe, err := mgr.Stat(r.Context(), path)
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(fe)
	}
}

// fileWriteHandler handles POST /api/v1/files (create directory or write file)
//
//	JSON body: {"path":"...","type":"file"|"dir","content":"<base64 or plain text>"}
func fileWriteHandler(mgr *fileService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Path    string `json:"path"`
			Type    string `json:"type"` // "file" or "dir"
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		if req.Path == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "path is required"))
			return
		}

		source := r.RemoteAddr
		switch req.Type {
		case "dir":
			if err := mgr.Mkdir(r.Context(), req.Path, source); err != nil {
				writeAppError(w, err)
				return
			}
		default: // "file" or empty
			if err := mgr.Write(r.Context(), req.Path, []byte(req.Content), source); err != nil {
				writeAppError(w, err)
				return
			}
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// fileDeleteHandler handles DELETE /api/v1/files
// Query parameters: path (required), recursive (optional bool)
func fileDeleteHandler(mgr *fileService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "path query parameter is required"))
			return
		}
		recursive := r.URL.Query().Get("recursive") == "true"

		source := r.RemoteAddr
		if err := mgr.Remove(r.Context(), path, recursive, source); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// fileMoveHandler handles PUT /api/v1/files/move
// JSON body: {"src":"...","dst":"..."}
func fileMoveHandler(mgr *fileService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		if req.Src == "" || req.Dst == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "src and dst are required"))
			return
		}

		if err := mgr.Move(r.Context(), req.Src, req.Dst, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// fileCopyHandler handles PUT /api/v1/files/copy
// JSON body: {"src":"...","dst":"..."}
func fileCopyHandler(mgr *fileService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		if req.Src == "" || req.Dst == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "src and dst are required"))
			return
		}

		if err := mgr.Copy(r.Context(), req.Src, req.Dst, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// fileReadHandler handles GET /api/v1/files/read
// Query parameter: path (required)
func fileReadHandler(mgr *fileService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "path query parameter is required"))
			return
		}

		data, err := mgr.Read(r.Context(), path)
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"path":    path,
			"content": string(data),
		})
	}
}

// fileDispatchHandler routes /api/v1/files/ sub-paths.
func fileDispatchHandler(mgr *fileService.Manager) http.Handler {
	statH := fileStatHandler(mgr)
	moveH := fileMoveHandler(mgr)
	copyH := fileCopyHandler(mgr)
	readH := fileReadHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip prefix /api/v1/files/ to get sub-path.
		sub := r.URL.Path[len("/api/v1/files/"):]
		switch sub {
		case "stat":
			statH.ServeHTTP(w, r)
		case "move":
			moveH.ServeHTTP(w, r)
		case "copy":
			copyH.ServeHTTP(w, r)
		case "read":
			readH.ServeHTTP(w, r)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
}
