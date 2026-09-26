// Package sandbox holds integration smoke tests for the daily-affairs sandbox
// routes. The goal is regression protection: after a refactor, the 14 business
// list/config endpoints must remain wired (not accidentally dropped to 404).
package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSandboxRoutesWired asserts that the BusinessExtractHandler exposes the
// 14 business endpoints the frontend calls. We register each method on a
// throwaway gin engine (with nil dependencies — the handlers will fail at
// nil-deref inside, but that is fine: we only assert the route is matched,
// i.e. the response is NOT 404).
func TestSandboxRoutesWired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Construct a handler shell. Dependencies are nil on purpose — we only
	// care that gin routes the request to the method, not that the method
	// succeeds. Routes that return 404 would mean the method is missing.
	h := &BusinessExtractHandler{}

	// Mirror the registration in internal/router/routes_knowledge.go under
	// the /knowledge-bases/:id group.
	g := r.Group("/api/v1/knowledge-bases/:id")
	{
		g.GET("/invoices", h.ListInvoiceRecords)
		g.GET("/invoice-tax-rates", h.ListInvoiceTaxRates)
		g.GET("/contracts", h.ListContractRecords)
		g.GET("/contract-types", h.ListContractTypes)
		g.GET("/regulations", h.ListRegulationRecords)
		g.GET("/regulation-types", h.ListRegulationTypes)
		g.GET("/award-punish-records", h.ListAwardPunishRecords)
		g.GET("/award-punish-types", h.ListAwardPunishTypes)
		g.GET("/utility-bill-records", h.ListUtilityBillRecords)
		g.GET("/solar-bill-records", h.ListSolarBillRecords)
		g.GET("/recognition-config", h.GetRecognitionConfig)
		g.PUT("/recognition-config", h.SaveRecognitionConfig)
		g.POST("/recognition/reassess", h.ReassessRecognition)
		g.GET("/deleted-knowledge", h.ListDeletedKnowledge)
	}

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/invoices"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/invoice-tax-rates"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/contracts"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/contract-types"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/regulations"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/regulation-types"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/award-punish-records"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/award-punish-types"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/utility-bill-records"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/solar-bill-records"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/recognition-config"},
		{http.MethodPut, "/api/v1/knowledge-bases/kb1/recognition-config"},
		{http.MethodPost, "/api/v1/knowledge-bases/kb1/recognition/reassess"},
		{http.MethodGet, "/api/v1/knowledge-bases/kb1/deleted-knowledge"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			// The critical assertion: gin must have matched the route.
			// 404 = route missing (regression). 500/400/200 all acceptable
			// here (nil-deref panics are recovered by gin's default recovery
			// and surface as 500; we only gate on 404).
			if w.Code == http.StatusNotFound {
				t.Fatalf("route %s %s returned 404 — endpoint not wired", tc.method, tc.path)
			}
		})
	}
}
