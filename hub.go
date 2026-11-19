package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var repoRE = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*/[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

type Choice struct {
	Name string `json:"rfilename"`
	Size int64  `json:"size"`
	LFS  struct {
		SHA string `json:"sha256"`
	} `json:"lfs"`
}
type Catalog struct {
	Repo, Revision string
	Files          []Choice
}

func catalog(ctx context.Context, input string) (Catalog, error) {
	repo := strings.Trim(strings.TrimPrefix(strings.TrimSpace(input), "https://huggingface.co/"), "/")
	c := Catalog{Repo: repo}
	if !repoRE.MatchString(repo) {
		return c, fail("use a public Hugging Face owner/repository URL")
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", "https://huggingface.co/api/models/"+repo+"?blobs=true", nil)
	res, e := http.DefaultClient.Do(r)
	if e != nil {
		return c, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return c, fmt.Errorf("Hugging Face returned %s", res.Status)
	}
	var meta struct {
		Sha      string
		Siblings []Choice
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&meta); e != nil {
		return c, e
	}
	if len(meta.Sha) != 40 {
		return c, fail("missing repository revision")
	}
	c.Revision = meta.Sha
	for _, f := range meta.Siblings {
		if strings.HasSuffix(strings.ToLower(f.Name), ".gguf") {
			c.Files = append(c.Files, f)
		}
	}
	return c, nil
}
func (a *App) download(ctx context.Context, i Input) (any, error) {
	c, e := catalog(ctx, i.Repo)
	if e != nil {
		return nil, e
	}
	if c.Revision != i.Revision {
		return nil, fail("repository changed; refresh the file list")
	}
	var choice *Choice
	for _, f := range c.Files {
		if f.Name == i.Filename {
			choice = &f
			break
		}
	}
	if choice == nil {
		return nil, fail("choose a listed GGUF")
	}
	u := url.URL{Scheme: "https", Host: "huggingface.co", Path: "/" + c.Repo + "/resolve/" + c.Revision + "/" + choice.Name}
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("download: %s", res.Status)
	}
	dir := filepath.Join(a.dir, "downloads", id())
	if e = os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	good := false
	defer func() {
		if !good {
			os.RemoveAll(dir)
		}
	}()
	dest := filepath.Join(dir, filepath.Base(path.Base(choice.Name)))
	out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(out, hash), res.Body)
	ce := out.Close()
	if e != nil {
		return nil, e
	}
	if ce != nil {
		return nil, ce
	}
	if choice.Size > 0 && n != choice.Size || choice.LFS.SHA != "" && hex.EncodeToString(hash.Sum(nil)) != choice.LFS.SHA {
		return nil, fail("download failed integrity check")
	}
	a.Lock()
	defer a.Unlock()
	f, e := a.register(dest, c.Repo+"@"+c.Revision)
	good = e == nil
	return f, e
}
