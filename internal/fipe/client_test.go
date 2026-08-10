package fipe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientReturnsTheBodyOfAMissingQuote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"veículo não encontrado para a referência informada"}`))
	}))
	defer server.Close()

	body, err := NewHTTPFetcher().FetchPage(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("a year fipe does not publish must not read as an outage: %v", err)
	}
	if !strings.Contains(body, "não encontrado") {
		t.Errorf("body = %q, want the not-found payload so ParseQuote can reject it", body)
	}
}

func TestClientTreatsAServerFaultAsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, err := NewHTTPFetcher().FetchPage(context.Background(), server.URL); err == nil {
		t.Error("a 500 must surface as an error so the outage is logged")
	}
}

func TestClientReturnsAHealthyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(readFixture(t, "value-5760-2014.json")))
	}))
	defer server.Close()

	body, err := NewHTTPFetcher().FetchPage(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	q, err := ParseQuote(strings.NewReader(body))
	if err != nil || q.Code != "810059-4" {
		t.Errorf("quote = %+v, err = %v", q, err)
	}
}
