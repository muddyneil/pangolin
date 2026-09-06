package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchOneUsesFallbackAfterPrimaryFailsFast(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "not found", http.StatusNotFound)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"proxies":[{"name":"fb-1","type":"ss","server":"127.0.0.1","port":443,"cipher":"aes-256-gcm","password":"pass"}]}`))
	}))
	defer fallback.Close()

	result := fetchOne(context.Background(), Source{Name: "test", Primary: primary.URL, Fallbacks: []string{fallback.URL}})
	if result.Err != nil {
		t.Fatalf("expected fallback result, got error: %v", result.Err)
	}
	if len(result.Proxies) != 1 || result.Proxies[0].Name != "fb-1" {
		t.Fatalf("unexpected fallback proxies: %#v", result.Proxies)
	}
}

func TestFetchFailsFastOnClientError(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		http.Error(writer, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	data, err := fetchWithRetry(context.Background(), server.URL)
	if err == nil {
		t.Fatalf("expected error, got data %q", data)
	}
	if calls != 1 {
		t.Fatalf("404 was retried %d times, want 1", calls)
	}
}

func TestFetchRetriesTransientHTTPStatuses(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls++
				if calls == 1 {
					http.Error(writer, "temporary failure", status)
					return
				}
				_, _ = writer.Write([]byte("ok"))
			}))
			defer server.Close()

			data, err := fetchWithRetry(context.Background(), server.URL)
			if err != nil || string(data) != "ok" {
				t.Fatalf("expected retry success, got data %q, error %v", data, err)
			}
			if calls != 2 {
				t.Fatalf("status %d was called %d times, want 2", status, calls)
			}
		})
	}
}

func TestFetchRetryStopsWhenContextIsCanceledDuringBackoff(t *testing.T) {
	calls := 0
	firstRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if calls == 1 {
			close(firstRequest)
		}
		http.Error(writer, "temporary failure", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-firstRequest
		cancel()
	}()
	started := time.Now()
	_, err := fetchWithRetry(ctx, server.URL)
	if err == nil {
		t.Fatal("expected canceled retry to return an error")
	}
	if calls != 1 {
		t.Fatalf("request was retried after cancellation: %d calls", calls)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("cancellation did not interrupt backoff quickly: %v", elapsed)
	}
}

func TestFetchOneUsesFallbackAfterPrimaryWindowExpires(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(4 * time.Second)
		_, _ = writer.Write([]byte(`{"proxies":[{"name":"primary","type":"ss","server":"127.0.0.1","port":443,"cipher":"aes-256-gcm","password":"pass"}]}`))
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"proxies":[{"name":"fallback","type":"ss","server":"127.0.0.1","port":443,"cipher":"aes-256-gcm","password":"pass"}]}`))
	}))
	defer fallback.Close()

	result := fetchOne(context.Background(), Source{Name: "test", Primary: primary.URL, Fallbacks: []string{fallback.URL}})
	if result.Err != nil || len(result.Proxies) != 1 || result.Proxies[0].Name != "fallback" {
		t.Fatalf("expected fallback after primary window, got %#v", result)
	}
}

func TestFetchAndNormalize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"proxies":[{"name":"hk-1","type":"ss","server":"127.0.0.1","port":443,"cipher":"aes-256-gcm","password":"pass"},{"name":"bad","type":"unknown","server":"x","port":1}]}`))
	}))
	defer server.Close()
	results := FetchAll(context.Background(), []Source{{Name: "test", Primary: server.URL}})
	if len(results) != 1 || results[0].Err != nil || len(results[0].Proxies) != 1 {
		t.Fatalf("unexpected result: %#v", results)
	}
}

func TestMergeDeduplicatesAndRenames(t *testing.T) {
	items := []FetchResult{{Proxies: []Proxy{{Name: "same", Type: "ss", Server: "a", Port: 1}}}, {Proxies: []Proxy{{Name: "same", Type: "ss", Server: "b", Port: 2}, {Name: "duplicate", Type: "ss", Server: "a", Port: 1}}}}
	merged := Merge(items)
	if len(merged) != 2 || merged[1].Name != "same-2" {
		t.Fatalf("unexpected merged nodes: %#v", merged)
	}
}
