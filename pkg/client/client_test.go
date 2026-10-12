package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/izhw/ai-agent/contract"
)

func TestRunStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Run-ID", "run-1")
		if _, err := w.Write(
			[]byte(
				"id: 1\nevent: message.completed\n" +
					"data: {\"run_id\":\"run-1\",\"type\":\"message.completed\",\"content\":\"hello\"}\n\n",
			),
		); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL}
	run, stream, err := client.Start(context.Background(), "assistant", "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if run.ID != "run-1" {
		t.Fatalf("run id %q", run.ID)
	}
	event, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != contract.EventMessageCompleted || event.Content != "hello" {
		t.Fatalf("unexpected %#v", event)
	}
}
