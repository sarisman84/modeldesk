package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var assets embed.FS

type File struct {
	ID, Path, Source string
	Size             int64
}
type Profile struct {
	ID, Name, File, Engine, Binary, SwapBinary string
	Port, Context, Threads, GPU                int
	Args                                       []string
}
type Server struct{ ID, Name, URL, Engine, Config string }
type Library struct {
	Files    map[string]File
	Profiles map[string]Profile
	Servers  map[string]Server
}
type App struct {
	sync.Mutex
	dir   string
	lib   Library
	procs map[string]*exec.Cmd
	exits map[string]string
}
type Input struct {
	ID, Path, Name, Repo, Revision, Filename, Text, Hash string
	Delete                                               bool
	Profile                                              Profile
	Server                                               Server
}

func id() string {
	var b [12]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func openApp(dir string) (*App, error) {
	var err error
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	a := &App{dir: dir, lib: Library{map[string]File{}, map[string]Profile{}, map[string]Server{}}, procs: map[string]*exec.Cmd{}, exits: map[string]string{}}
	b, e := os.ReadFile(filepath.Join(dir, "library.json"))
	if e == nil {
		e = json.Unmarshal(b, &a.lib)
		if e == nil && (a.lib.Files == nil || a.lib.Profiles == nil || a.lib.Servers == nil) {
			e = errors.New("invalid library")
		}
	}
	if os.IsNotExist(e) {
		e = nil
	}
	return a, e
}
func replace(path string, b []byte, mode os.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".modeldesk-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
func (a *App) save() error {
	b, e := json.MarshalIndent(a.lib, "", "  ")
	if e != nil {
		return e
	}
	return replace(filepath.Join(a.dir, "library.json"), b, 0600)
}
func fail(s string) error { return errors.New(s) }
func (a *App) register(path, source string) (File, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".gguf") {
		return File{}, fail("choose an absolute .gguf file path")
	}
	p, e := filepath.EvalSymlinks(path)
	if e != nil {
		return File{}, e
	}
	fi, e := os.Stat(p)
	if e != nil {
		return File{}, e
	}
	if !fi.Mode().IsRegular() {
		return File{}, fail("not a regular file")
	}
	for _, f := range a.lib.Files {
		info, e := os.Stat(f.Path)
		if e == nil && os.SameFile(info, fi) {
			return f, nil
		}
	}
	handle, e := os.Open(p)
	if e != nil {
		return File{}, e
	}
	defer handle.Close()
	var magic [4]byte
	if _, e = handle.Read(magic[:]); e != nil || string(magic[:]) != "GGUF" {
		return File{}, fail("file does not have a GGUF header")
	}
	f := File{id(), p, source, fi.Size()}
	a.lib.Files[f.ID] = f
	if e = a.save(); e != nil {
		delete(a.lib.Files, f.ID)
	}
	return f, e
}
func endpoint(s string) bool {
	u, e := url.Parse(s)
	if e != nil || u.User != nil || u.Scheme != "http" || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return false
	}
	return u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
}
func (a *App) action(ctx context.Context, op string, i Input) (any, error) {
	switch op {
	case "catalog":
		return catalog(ctx, i.Repo)
	case "download":
		return a.download(ctx, i)
	case "status":
		return a.status(ctx, i.ID)
	}
	a.Lock()
	defer a.Unlock()
	switch op {
	case "library":
		running := map[string]bool{}
		for k := range a.procs {
			running[k] = true
		}
		b, e := json.Marshal(map[string]any{"library": a.lib, "running": running, "exits": a.exits})
		return json.RawMessage(b), e
	case "import":
		return a.register(i.Path, "Imported")
	case "removeFile":
		f, ok := a.lib.Files[i.ID]
		if !ok {
			return nil, fail("unknown file")
		}
		for _, p := range a.lib.Profiles {
			if p.File == f.ID {
				return nil, fail("remove referencing profiles first")
			}
		}
		if i.Delete {
			if i.Path != f.Path {
				return nil, fail("path confirmation mismatch")
			}
			s, e := os.Lstat(f.Path)
			if e != nil {
				return nil, e
			}
			if !s.Mode().IsRegular() || s.Size() != f.Size {
				return nil, fail("file changed; forget it and import again")
			}
			if e = os.Remove(f.Path); e != nil {
				return nil, e
			}
		}
		delete(a.lib.Files, i.ID)
		return nil, a.save()
	case "profile":
		p := i.Profile
		if _, ok := a.lib.Files[p.File]; !ok || strings.TrimSpace(p.Name) == "" || p.Port < 1 || p.Port > 65535 || p.Context < 0 || p.Threads < 0 || p.GPU < 0 || (p.Engine != "cpp" && p.Engine != "swap") {
			return nil, fail("invalid profile settings")
		}
		if p.ID == "" {
			p.ID = id()
		} else if _, ok := a.lib.Profiles[p.ID]; !ok {
			return nil, fail("unknown profile")
		}
		if a.procs[p.ID] != nil {
			return nil, fail("stop the profile before editing")
		}
		a.lib.Profiles[p.ID] = p
		return p, a.save()
	case "removeProfile":
		if a.procs[i.ID] != nil {
			return nil, fail("stop the profile first")
		}
		delete(a.lib.Profiles, i.ID)
		return nil, a.save()
	case "server":
		s := i.Server
		if !endpoint(s.URL) || strings.TrimSpace(s.Name) == "" || (s.Engine != "cpp" && s.Engine != "swap") {
			return nil, fail("enter a name and a local HTTP origin")
		}
		s.URL = strings.TrimRight(s.URL, "/")
		if s.Config != "" {
			p, e := configPath(s)
			if e != nil {
				return nil, e
			}
			s.Config = p
		}
		s.ID = id()
		a.lib.Servers[s.ID] = s
		return s, a.save()
	case "disconnect":
		delete(a.lib.Servers, i.ID)
		return nil, a.save()
	case "config", "apply":
		return a.config(op, i)
	case "launch":
		return nil, a.launch(i.ID)
	case "stop":
		if c := a.procs[i.ID]; c != nil {
			return nil, terminate(c)
		}
		return nil, fail("not running")
	}
	return nil, fail("unknown action")
}
func (a *App) handler(host string) http.Handler {
	root, _ := fs.Sub(assets, "web")
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "unexpected host", 403)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Origin") != "http://"+host {
			http.Error(w, "same-origin JSON POST required", 403)
			return
		}
		var i Input
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&i); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		v, e := a.action(r.Context(), strings.TrimPrefix(r.URL.Path, "/api/"), i)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	})
}
func main() {
	d, e := os.UserConfigDir()
	if e != nil {
		log.Fatal(e)
	}
	dir := flag.String("data", filepath.Join(d, "modeldesk"), "library and downloads directory")
	port := flag.Int("port", 43120, "local dashboard port")
	browse := flag.Bool("open", true, "open default browser")
	flag.Parse()
	l, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if e != nil {
		log.Fatal(e)
	}
	a, e := openApp(*dir)
	if e != nil {
		log.Fatal(e)
	}
	addr := l.Addr().String()
	fmt.Println("Modeldesk: http://" + addr)
	if *browse {
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.Command("rundll32", "url.dll,FileProtocolHandler", "http://"+addr)
		} else if runtime.GOOS == "darwin" {
			c = exec.Command("open", "http://"+addr)
		} else {
			c = exec.Command("xdg-open", "http://"+addr)
		}
		go c.Run()
	}
	srv := http.Server{Handler: a.handler(addr), ReadHeaderTimeout: 5 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	go func() {
		<-ctx.Done()
		a.Lock()
		for _, c := range a.procs {
			terminate(c)
		}
		a.Unlock()
		srv.Close()
	}()
	if e = srv.Serve(l); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
