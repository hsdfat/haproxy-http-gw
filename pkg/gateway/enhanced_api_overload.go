// Copyright 2019 HAProxy Technologies LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/haproxytech/kubernetes-ingress/pkg/overload"
)

// OverloadRuleRequest is the body for POST /api/frontends/{id}/overload.
type OverloadRuleRequest struct {
	Path  string `json:"path"`
	Limit int64  `json:"limit"`
}

type overloadResponse struct {
	Success bool             `json:"success"`
	Message string           `json:"message,omitempty"`
	Rule    *overload.Rule   `json:"rule,omitempty"`
	Rules   []overload.Rule  `json:"rules,omitempty"`
	Stats   []overload.StatsLine `json:"stats,omitempty"`
}

func (a *EnhancedAPIServer) writeOverload(w http.ResponseWriter, status int, body overloadResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// addOverloadRule handles POST /api/frontends/{id}/overload
func (a *EnhancedAPIServer) addOverloadRule(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	if frontendID == "" {
		a.writeOverload(w, http.StatusBadRequest, overloadResponse{Message: "frontend ID is required"})
		return
	}
	var req OverloadRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeOverload(w, http.StatusBadRequest, overloadResponse{Message: "invalid request body: " + err.Error()})
		return
	}
	rule, err := a.frontendManager.AddOverloadRule(frontendID, req.Path, req.Limit)
	if err != nil {
		a.writeOverload(w, http.StatusBadRequest, overloadResponse{Message: err.Error()})
		return
	}
	logger.Infof("Overload rule added: frontend=%s path=%s limit=%d", frontendID, rule.Path, rule.Limit)
	a.writeOverload(w, http.StatusOK, overloadResponse{Success: true, Rule: &rule})
}

// listOverloadRules handles GET /api/frontends/{id}/overload
func (a *EnhancedAPIServer) listOverloadRules(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	rules, err := a.frontendManager.ListOverloadRules(frontendID)
	if err != nil {
		a.writeOverload(w, http.StatusBadRequest, overloadResponse{Message: err.Error()})
		return
	}
	a.writeOverload(w, http.StatusOK, overloadResponse{Success: true, Rules: rules})
}

// deleteOverloadRule handles DELETE /api/frontends/{id}/overload?path=...
func (a *EnhancedAPIServer) deleteOverloadRule(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	path := r.URL.Query().Get("path")
	if path == "" {
		a.writeOverload(w, http.StatusBadRequest, overloadResponse{Message: "query parameter 'path' is required"})
		return
	}
	if err := a.frontendManager.DeleteOverloadRule(frontendID, path); err != nil {
		a.writeOverload(w, http.StatusNotFound, overloadResponse{Message: err.Error()})
		return
	}
	logger.Infof("Overload rule deleted: frontend=%s path=%s", frontendID, path)
	a.writeOverload(w, http.StatusOK, overloadResponse{Success: true})
}

// getOverloadStats handles GET /api/frontends/{id}/overload/stats
func (a *EnhancedAPIServer) getOverloadStats(w http.ResponseWriter, r *http.Request) {
	frontendID := r.PathValue("id")
	stats, err := a.frontendManager.GetOverloadStats(frontendID)
	if err != nil {
		a.writeOverload(w, http.StatusBadRequest, overloadResponse{Message: err.Error()})
		return
	}
	a.writeOverload(w, http.StatusOK, overloadResponse{Success: true, Stats: stats})
}
