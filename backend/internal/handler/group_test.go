package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/0x2E/fusion/internal/model"
	"github.com/gin-gonic/gin"
)

func TestReorderGroupsEndpoint(t *testing.T) {
	h, st := newAnonTestHandler(t, nil)

	g1, err := st.CreateGroup("Alpha")
	if err != nil {
		t.Fatalf("CreateGroup Alpha: %v", err)
	}
	g2, err := st.CreateGroup("Beta")
	if err != nil {
		t.Fatalf("CreateGroup Beta: %v", err)
	}

	r := h.SetupRouter()

	w := performRequest(
		r,
		http.MethodPatch,
		"/api/groups/-/order",
		mustJSONBody(t, gin.H{"group_ids": []int64{g2.ID, 1, g1.ID}}),
		map[string]string{"Content-Type": "application/json"},
	)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH /api/groups/-/order status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []model.Group `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Total != 3 || len(resp.Data) != 3 {
		t.Fatalf("unexpected response count total=%d len=%d", resp.Total, len(resp.Data))
	}
	wantIDs := []int64{g2.ID, 1, g1.ID}
	for i, id := range wantIDs {
		if resp.Data[i].ID != id {
			t.Fatalf("resp.Data[%d].ID = %d, want %d", i, resp.Data[i].ID, id)
		}
	}

	getW := performRequest(r, http.MethodGet, "/api/groups", nil, nil)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET /api/groups status = %d", getW.Code)
	}
	var listed struct {
		Data []model.Group `json:"data"`
	}
	if err := json.Unmarshal(getW.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal GET /api/groups: %v", err)
	}
	for i, id := range wantIDs {
		if listed.Data[i].ID != id {
			t.Fatalf("listed.Data[%d].ID = %d, want %d", i, listed.Data[i].ID, id)
		}
	}
}

func TestReorderGroupsValidation(t *testing.T) {
	h, _ := newAnonTestHandler(t, nil)
	r := h.SetupRouter()

	tests := []struct {
		name     string
		body     any
		wantCode int
	}{
		{name: "missing group_ids", body: gin.H{}, wantCode: http.StatusBadRequest},
		{name: "empty group_ids", body: gin.H{"group_ids": []int64{}}, wantCode: http.StatusBadRequest},
		{name: "duplicate group_ids", body: gin.H{"group_ids": []int64{1, 1}}, wantCode: http.StatusBadRequest},
		{name: "unknown group_id", body: gin.H{"group_ids": []int64{99999}}, wantCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := performRequest(
				r,
				http.MethodPatch,
				"/api/groups/-/order",
				mustJSONBody(t, tt.body),
				map[string]string{"Content-Type": "application/json"},
			)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}
