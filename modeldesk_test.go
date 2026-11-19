package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedFilesAndConfigConflict(t *testing.T) {
	dir := t.TempDir()
	a, e := openApp(dir)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	call := func(op string, i Input) any {
		t.Helper()
		v, e := a.action(ctx, op, i)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	path := filepath.Join(dir, "shared.gguf")
	if e = os.WriteFile(path, []byte("GGUFfixture"), 0600); e != nil {
		t.Fatal(e)
	}
	f := call("import", Input{Path: path}).(File)
	p := Profile{Name: "long context", File: f.ID, Port: 8081, Engine: "cpp"}
	one := call("profile", Input{Profile: p}).(Profile)
	p.Name = "short context"
	two := call("profile", Input{Profile: p}).(Profile)
	call("removeProfile", Input{ID: one.ID})
	if _, e = a.action(ctx, "removeFile", Input{ID: f.ID, Path: path, Delete: true}); e == nil {
		t.Fatal("deleted referenced weights")
	}
	a, e = openApp(dir)
	if e != nil {
		t.Fatal(e)
	}
	call("removeProfile", Input{ID: two.ID})
	call("removeFile", Input{ID: f.ID})
	if _, e = os.Stat(path); e != nil {
		t.Fatal("forget removed the file")
	}
	cfg := filepath.Join(dir, "server.ini")
	os.WriteFile(cfg, []byte("[one]\n"), 0600)
	s := call("server", Input{Server: Server{Name: "test", URL: "http://127.0.0.1:8080", Engine: "cpp", Config: cfg}}).(Server)
	v := call("config", Input{ID: s.ID}).(map[string]string)
	update := Input{ID: s.ID, Hash: v["hash"], Text: "[two]\n"}
	backup := call("apply", update).(string)
	b, e := os.ReadFile(backup)
	if e != nil || string(b) != "[one]\n" {
		t.Fatal("missing original backup")
	}
	if _, e = a.action(ctx, "apply", update); e == nil {
		t.Fatal("accepted stale edit")
	}
}
func TestOfflineIsUnknown(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			io.WriteString(w, `{"data":[{"id":"demo"}]}`)
		case "/running":
			io.WriteString(w, `{"running":[{"model":"demo","state":"ready"}]}`)
		case "/metrics":
			io.WriteString(w, "llamacpp:requests_processing 2\n")
		}
	}))
	defer s.Close()
	a, _ := openApp(t.TempDir())
	a.lib.Servers["s"] = Server{URL: s.URL, Engine: "swap"}
	h, _ := a.status(context.Background(), "s")
	if h.Models["demo"] != "ready" || h.Metrics["llamacpp:requests_processing"] != 2 {
		t.Fatal(h)
	}
	s.Close()
	h, _ = a.status(context.Background(), "s")
	if h.State != "Unknown" {
		t.Fatal(h)
	}
}
