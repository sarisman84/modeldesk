package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func configPath(s Server) (string, error) {
	ext := strings.ToLower(filepath.Ext(s.Config))
	if !filepath.IsAbs(s.Config) || !(s.Engine == "cpp" && ext == ".ini" || s.Engine == "swap" && (ext == ".yaml" || ext == ".yml")) {
		return "", fail("select an absolute path to a llama-swap YAML or llama.cpp preset INI")
	}
	return filepath.EvalSymlinks(s.Config)
}
func (a *App) config(op string, i Input) (any, error) {
	s, ok := a.lib.Servers[i.ID]
	if !ok || s.Config == "" {
		return nil, fail("no editable config attached")
	}
	p, e := configPath(s)
	if e != nil {
		return nil, e
	}
	fi, e := os.Stat(p)
	if e != nil {
		return nil, e
	}
	if !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return nil, fail("expected a regular config file under 1 MiB")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return nil, e
	}
	h := fmt.Sprintf("%x", sha256.Sum256(b))
	if op == "config" {
		return map[string]string{"text": string(b), "hash": h}, nil
	}
	if i.Hash != h {
		return nil, fail("file changed since preview; reopen the editor")
	}
	if strings.TrimSpace(i.Text) == "" || len(i.Text) > 1<<20 {
		return nil, fail("empty or oversized config")
	}
	backup := p + ".modeldesk-" + id() + ".bak"
	if e = os.WriteFile(backup, b, 0600); e != nil {
		return nil, e
	}
	if e = replace(p, []byte(i.Text), fi.Mode().Perm()); e != nil {
		return nil, e
	}
	return backup, nil
}
func (a *App) launch(k string) error {
	p, ok := a.lib.Profiles[k]
	if !ok {
		return fail("unknown profile")
	}
	if a.procs[k] != nil {
		return fail("already running")
	}
	f := a.lib.Files[p.File]
	if _, e := os.Stat(f.Path); e != nil {
		return e
	}
	port := strconv.Itoa(p.Port)
	addr := "127.0.0.1:" + port
	l, e := net.Listen("tcp4", addr)
	if e != nil {
		return e
	}
	l.Close()
	binary := p.Binary
	if binary == "" {
		binary = "llama-server"
	}
	args := []string{"-m", f.Path, "--alias", p.Name, "--host", "127.0.0.1", "--port", port, "--metrics", "--n-gpu-layers", strconv.Itoa(p.GPU)}
	if p.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(p.Threads))
	}
	if p.Context > 0 {
		args = append(args, "-c", strconv.Itoa(p.Context))
	}
	args = append(args, p.Args...)
	if p.Engine == "swap" {
		args[7] = "${PORT}"
		words := append([]string{binary}, args...)
		for n, w := range words {
			if strings.ContainsAny(w, "\"\r\n") {
				return fail("quotes and newlines in swap arguments are unsupported")
			}
			words[n] = "\"" + w + "\""
		}
		cfg := map[string]any{"models": map[string]any{p.Name: map[string]string{"cmd": strings.Join(words, " ")}}}
		b, _ := json.Marshal(cfg)
		path := filepath.Join(a.dir, k+".yaml")
		if e = replace(path, b, 0600); e != nil {
			return e
		}
		binary = p.SwapBinary
		if binary == "" {
			binary = "llama-swap"
		}
		args = []string{"-config", path, "-listen", addr}
	}
	c := exec.Command(binary, args...)
	prepare(c)
	out, e := os.OpenFile(filepath.Join(a.dir, k+".log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	c.Stderr = out
	c.Stdout = out
	if e = c.Start(); e != nil {
		out.Close()
		return e
	}
	a.procs[k] = c
	delete(a.exits, k)
	go func() {
		e := c.Wait()
		out.Close()
		a.Lock()
		defer a.Unlock()
		delete(a.procs, k)
		if e != nil {
			a.exits[k] = fmt.Sprintf("%v · log: %s", e, out.Name())
		}
	}()
	return nil
}

var localHTTP = &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func get(ctx context.Context, u string) ([]byte, int, error) {
	r, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return nil, 0, e
	}
	res, e := localHTTP.Do(r)
	if e != nil {
		return nil, 0, e
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	return b, res.StatusCode, e
}

type named struct {
	ID, Model, State, Proxy string
	Status                  struct{ Value string }
}
type Health struct {
	State, Detail string
	Models        map[string]string
	Metrics       map[string]float64
	Checked       time.Time
}

func (a *App) status(ctx context.Context, k string) (Health, error) {
	a.Lock()
	s, ok := a.lib.Servers[k]
	if !ok {
		p, found := a.lib.Profiles[k]
		if !found || a.procs[k] == nil {
			a.Unlock()
			return Health{}, fail("no active connection")
		}
		s = Server{Engine: p.Engine, URL: fmt.Sprintf("http://127.0.0.1:%d", p.Port)}
	}
	a.Unlock()
	h := Health{State: "Unknown", Checked: time.Now(), Models: map[string]string{}, Metrics: map[string]float64{}}
	b, code, e := get(ctx, s.URL+"/v1/models")
	if e != nil {
		h.Detail = e.Error()
		return h, nil
	}
	if code != 200 {
		h.Detail = fmt.Sprintf("Model listing returned HTTP %d", code)
		if code == 503 {
			h.State = "Starting / unavailable"
		}
		return h, nil
	}
	var list struct{ Data []named }
	if json.Unmarshal(b, &list) != nil || list.Data == nil {
		h.Detail = "Unexpected model listing"
		return h, nil
	}
	h.State = "Online"
	for _, m := range list.Data {
		h.Models[m.ID] = "Available · load state unknown"
	}
	if s.Engine == "swap" {
		var live struct{ Running []named }
		if b, c, _ := get(ctx, s.URL+"/running"); c == 200 && json.Unmarshal(b, &live) == nil && live.Running != nil {
			for k := range h.Models {
				h.Models[k] = "Not running locally"
			}
			for _, m := range live.Running {
				h.Models[m.Model] = m.State
				if strings.EqualFold(m.State, "ready") && endpoint(m.Proxy) {
					readMetrics(ctx, strings.TrimRight(m.Proxy, "/"), m.Model+" / ", h.Metrics)
				}
			}
		}
	} else {
		var router struct{ Data []named }
		if b, c, _ := get(ctx, s.URL+"/models"); c == 200 && json.Unmarshal(b, &router) == nil {
			for _, m := range router.Data {
				if m.Status.Value != "" {
					h.Models[m.ID] = m.Status.Value
				}
			}
		}
		if len(list.Data) == 1 {
			m := list.Data[0].ID
			if strings.Contains(h.Models[m], "unknown") {
				if b, c, _ := get(ctx, s.URL+"/health"); c == 200 {
					var v struct{ Status string }
					if json.Unmarshal(b, &v) == nil && v.Status == "ok" {
						h.Models[m] = "Ready"
					}
				}
			}
		}
	}
	readMetrics(ctx, s.URL, "", h.Metrics)
	return h, nil
}
func readMetrics(ctx context.Context, base, prefix string, metrics map[string]float64) {
	if b, c, _ := get(ctx, base+"/metrics"); c == 200 {
		scan := bufio.NewScanner(strings.NewReader(string(b)))
		for scan.Scan() {
			line := scan.Text()
			if strings.HasPrefix(line, "#") {
				continue
			}
			v := strings.Fields(line)
			if len(v) < 2 {
				continue
			}
			key := strings.ToLower(v[0])
			if strings.Contains(key, "request") || strings.Contains(key, "tokens_per_second") || strings.Contains(key, "tokens_seconds") {
				if val, e := strconv.ParseFloat(v[1], 64); e == nil && val >= 0 && val < 1e300 {
					metrics[prefix+v[0]] = val
				}
			}
		}
	}
}
