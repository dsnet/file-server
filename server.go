package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	//go:embed static/css/main.css
	mainCSS string

	//go:embed static/html/main.html
	mainHTML string
	//go:embed static/html/body.html
	bodyHTML string

	//go:embed static/js/files.js
	filesJS string
	//go:embed static/js/operations.js
	operationsJS string
	//go:embed static/js/buttons.js
	buttonsJS string
	//go:embed static/js/format.js
	formatJS string
)

type Server struct {
	root fs.FS

	hideRx  *regexp.Regexp
	denyRx  *regexp.Regexp
	indexRx *regexp.Regexp

	sendfile bool
	verbose  bool
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Never cache the server results. Consider it dynamically changing.
	w.Header().Set("Cache-Control", "no-cache, no-store, no-transform, must-revalidate, private, max-age=0")

	// For simplicity, always deal with clean paths that are absolute.
	// If the path had a trailing slash, preserve it.
	hadSlashSuffix := strings.HasSuffix(r.URL.Path, "/")
	r.URL.Path = "/" + strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if !strings.HasSuffix(r.URL.Path, "/") && hadSlashSuffix {
		r.URL.Path += "/"
	}

	// Log the request.
	if s.verbose {
		log.Printf("%s %s", r.Method, r.URL.Path)
	}

	// Verify that the file exists.
	f, err := s.root.Open(filepath.Join(".", filepath.FromSlash(r.URL.Path)))
	if err != nil {
		httpError(w, r, err)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		httpError(w, r, err)
		return
	}

	// Check that there is a trailing slash for only directories.
	if fi.IsDir() != strings.HasSuffix(r.URL.Path, "/") {
		if fi.IsDir() {
			relativeRedirect(w, r, path.Base(r.URL.Path)+"/") // directories always have slash suffix
			return
		} else {
			relativeRedirect(w, r, "../"+path.Base(r.URL.Path)) // files never have slash suffix
			return
		}
	}

	// Reject paths that match the deny pattern.
	if regexpMatch(s.denyRx, r.URL.Path) {
		httpError(w, r, os.ErrPermission)
		return
	}

	// Serve either a directory or a file.
	if fi.IsDir() {
		s.serveDirectory(w, r, s.root, f)
	} else {
		s.serveFile(w, r, f, fi.ModTime(), true)
	}
}

func (s *Server) serveDirectory(w http.ResponseWriter, r *http.Request, dir fs.FS, f fs.File) {
	// Read the directory entries, resolving any symbolic links,
	// and sorting all the entries by name.
	fd, ok := f.(fs.ReadDirFile)
	if !ok {
		httpError(w, r, os.ErrInvalid)
		return
	}
	fes, err := fd.ReadDir(0)
	if err != nil {
		httpError(w, r, err)
		return
	}

	type fileInfo struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Date int64  `json:"date"` // seconds since Unix epoch
	}
	fis := []fileInfo{}
	for _, fe := range fes {
		// Obtain the fs.FileInfo, resolving symbolic links if necessary.
		var fi fs.FileInfo
		if fe.Type()&os.ModeSymlink == 0 {
			fi, _ = fe.Info()
		} else {
			fi, _ = fs.Stat(dir, filepath.Join(".", filepath.FromSlash(r.URL.Path), fe.Name()))
		}
		if fi == nil {
			continue
		}

		// Check whether to hide or specially handle this file.
		urlPath := r.URL.Path + "/" + fi.Name()
		if regexpMatch(s.hideRx, urlPath) || regexpMatch(s.denyRx, urlPath) {
			continue
		}
		if regexpMatch(s.indexRx, urlPath) {
			f, err := dir.Open(filepath.Join(".", filepath.FromSlash(r.URL.Path), fi.Name()))
			if err != nil {
				httpError(w, r, err)
				return
			}
			defer f.Close()
			r.URL.Path = urlPath
			s.serveFile(w, r, f, fi.ModTime(), false)
			return
		}

		name := fi.Name()
		if fi.IsDir() {
			name += "/"
		}
		var size int64
		if fi.Mode().IsRegular() {
			size = fi.Size()
		}
		fis = append(fis, fileInfo{Name: name, Size: size, Date: fi.ModTime().Unix()})
	}

	// Format the list of files and folders.
	scripts := []string{filesJS, operationsJS + formatJS + buttonsJS}
	fileInfos, err := json.Marshal(fis)
	if err != nil {
		httpError(w, r, err)
		return
	}
	scripts = append(scripts, "fileInfos = "+string(fileInfos)+";\n"+"reorderFiles(compareNames);\n")
	body := bodyHTML
	body = strings.Replace(body, "{{.Script}}", "\n"+strings.Join(scripts, "\n"), 1)
	renderHTML(w, r, body)
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, f fs.File, modTime time.Time, allowRedirect bool) {
	if allowRedirect && regexpMatch(s.indexRx, r.URL.Path) {
		relativeRedirect(w, r, "./") // redirect to directory containing index.html
		return
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		b, err := io.ReadAll(f)
		if err != nil {
			httpError(w, r, err)
			return
		}
		rs = bytes.NewReader(b)
	}
	if !s.sendfile {
		rs = struct{ io.ReadSeeker }{rs} // drop ReadFrom method to avoid using sendfile syscall
	}
	http.ServeContent(w, r, r.URL.Path, modTime, rs)
}

func relativeRedirect(w http.ResponseWriter, r *http.Request, urlPath string) {
	if q := r.URL.RawQuery; q != "" {
		urlPath += "?" + q
	}
	w.Header().Set("Location", urlPath)
	w.WriteHeader(http.StatusMovedPermanently)
}

// regexpMatch is identical to r.MatchString(s),
// but reports false if r is nil.
func regexpMatch(r *regexp.Regexp, s string) bool {
	return r != nil && r.MatchString(s)
}

func renderHTML(w http.ResponseWriter, r *http.Request, body string) {
	var headers []string
	names := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	for i, name := range names {
		name += "/"
		urlString := "." + strings.Repeat("/..", len(names)-1-i)
		if !strings.HasSuffix(r.URL.Path, "/") {
			if i == len(names)-1 {
				name = strings.TrimSuffix(name, "/")
				urlString = path.Base(r.URL.Path)
			} else {
				urlString = strings.TrimSuffix(urlString, "/..")
			}
		}
		headers = append(headers, `<a href="`+html.EscapeString(urlString)+`">`+html.EscapeString(name)+`</a>`)
	}

	page := mainHTML
	page = strings.Replace(page, "{{.Title}}", html.EscapeString(path.Base(r.URL.Path)), 1)
	page = strings.Replace(page, "{{.Style}}", mainCSS, 1)
	page = strings.Replace(page, "{{.Header}}", strings.Join(headers, " "), 1)
	page = strings.Replace(page, "{{.Body}}", body, 1)
	io.WriteString(w, page)
}

func httpError(w http.ResponseWriter, r *http.Request, err error) {
	var code int
	switch {
	case os.IsNotExist(err):
		code = http.StatusNotFound
	case os.IsPermission(err):
		code = http.StatusForbidden
	default:
		code = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(code)

	renderHTML(w, r, http.StatusText(code)+": "+html.EscapeString(err.Error()))
}
