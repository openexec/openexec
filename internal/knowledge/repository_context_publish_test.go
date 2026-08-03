package knowledge

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestRepositoryContextPublisherUsesObservedVersionAndBoundedProjection(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization header = %q", request.Header.Get("Authorization"))
		}
		if requests == 1 {
			if request.Method != http.MethodGet {
				t.Fatalf("first method = %s", request.Method)
			}
			header := http.Header{}
			header.Set("ETag", `"old"`)
			return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		if request.Method != http.MethodPut || request.Header.Get("If-Match") != `"old"` {
			t.Fatalf("publish request = %s If-Match %q", request.Method, request.Header.Get("If-Match"))
		}
		body, _ := io.ReadAll(request.Body)
		text := string(body)
		if !strings.Contains(text, `"schema_version":1`) || strings.Contains(text, "source_content") {
			t.Fatalf("published body = %s", text)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	projection := RepositoryContextProjection{SchemaVersion: 1, SourceSystem: "openexec", OpenExecReference: OpenExecReference{ResourceVersion: "new"}}
	if err := (RepositoryContextPublisher{Client: client}).Publish(context.Background(), "https://console.example", "project-1", "secret", projection); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestRepositoryContextPublisherTreatsMissingProjectionAsFirstPublish(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		if request.Header.Get("If-Match") != "" {
			t.Fatal("first publish carried If-Match")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	if err := (RepositoryContextPublisher{Client: client}).Publish(context.Background(), "http://localhost:8080", "project-1", "", RepositoryContextProjection{}); err != nil {
		t.Fatal(err)
	}
}
