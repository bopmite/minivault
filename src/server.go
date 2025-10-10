package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type SetRequest struct {
	Value json.RawMessage `json:"value"`
}

func extractValue(body []byte, contentType string) []byte {
	if strings.Contains(contentType, "application/json") {
		var req SetRequest
		if err := json.Unmarshal(body, &req); err == nil {
			return []byte(req.Value)
		}
	}

	return body
}

func (v *Vault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	path := r.URL.Path
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	if strings.HasPrefix(path, "_sync/") {
		v.handleSync(w, r, path[6:])
		return
	}

	if path == "" {
		writeError(w, http.StatusBadRequest, "key required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		v.handleGet(w, r, path)
	case http.MethodPost:
		v.handlePost(w, r, path)
	case http.MethodPut:
		v.handlePut(w, r, path)
	case http.MethodDelete:
		v.handleDelete(w, r, path)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (v *Vault) handleGet(w http.ResponseWriter, _ *http.Request, key string) {
	val := Get(key)
	if val == nil {
		val = v.cluster.QuorumRead(key)
	}

	if val == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(val)
}

func (v *Vault) handlePost(w http.ResponseWriter, r *http.Request, key string) {
	if Get(key) != nil {
		writeError(w, http.StatusConflict, "exists")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	value := extractValue(body, r.Header.Get("Content-Type"))
	if len(value) == 0 {
		writeError(w, http.StatusBadRequest, "value required")
		return
	}

	if err := v.cluster.QuorumWrite(key, value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, http.StatusCreated, nil)
}

func (v *Vault) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	value := extractValue(body, r.Header.Get("Content-Type"))
	if len(value) == 0 {
		writeError(w, http.StatusBadRequest, "value required")
		return
	}

	if err := v.cluster.QuorumWrite(key, value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (v *Vault) handleDelete(w http.ResponseWriter, _ *http.Request, key string) {
	if !Delete(key) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	nodes := v.cluster.rendezvousHash(key, 3)
	for _, node := range nodes {
		if node != v.selfURL {
			go v.sendDelete(node, key)
		}
	}

	writeSuccess(w, http.StatusOK, nil)
}

func (v *Vault) handleSync(w http.ResponseWriter, r *http.Request, key string) {
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}

		var msg SyncMsg
		if err := json.Unmarshal(body, &msg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		Set(msg.Key, msg.Value)
		writeSuccess(w, http.StatusOK, nil)

	case http.MethodDelete:
		Delete(key)
		writeSuccess(w, http.StatusOK, nil)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (v *Vault) sendDelete(node, key string) {
	req, _ := http.NewRequest(http.MethodDelete, node+"/_sync/"+key, nil)
	resp, err := v.cluster.client.Do(req)

	if err == nil {
		resp.Body.Close()
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	resp := Response{Success: false, Error: msg}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeSuccess(w http.ResponseWriter, status int, data json.RawMessage) {
	resp := Response{Success: true, Data: data}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
