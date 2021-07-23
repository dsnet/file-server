// Copyright 2021, Joe Tsai. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	addr     = flag.String("addr", ":8080", "The network address to listen on.")
	hide     = flag.String("hide", "/[.][^/]+/?$", "Regular expression of file paths to hide.\nPaths matching this pattern are excluded from directory listings,\nbut direct requests for this path are still resolved.")
	deny     = flag.String("deny", "", "Regular expression of file paths to deny.\nPaths matching this pattern are excluded from directory listings\nand direct requests for this path report StatusForbidden.")
	index    = flag.String("index", "", "Regular expression of file paths to treat as index.html pages.\n(e.g., '/index[.]html$'; default none)")
	root     = flag.String("root", ".", "Directory to serve files from.")
	sendfile = flag.Bool("sendfile", true, "Allow the use of the sendfile syscall.")
	verbose  = flag.Bool("verbose", false, "Log every HTTP request.")

	hideRx  *regexp.Regexp
	denyRx  *regexp.Regexp
	indexRx *regexp.Regexp
)

func main() {
	// Process command line flags.
	var err error
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [OPTION]...\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(flag.CommandLine.Output(), "Invalid argument: %v\n\n", flag.Arg(0))
		flag.Usage()
		os.Exit(1)
	}
	if *hide != "" {
		hideRx, err = regexp.Compile(*hide)
		if err != nil {
			fmt.Fprintf(flag.CommandLine.Output(), "Invalid hide pattern: %v\n\n", *hide)
			flag.Usage()
			os.Exit(1)
		}
	}
	if *deny != "" {
		denyRx, err = regexp.Compile(*deny)
		if err != nil {
			fmt.Fprintf(flag.CommandLine.Output(), "Invalid deny pattern: %v\n\n", *deny)
			flag.Usage()
			os.Exit(1)
		}
	}
	if *index != "" {
		indexRx, err = regexp.Compile(*index)
		if err != nil {
			fmt.Fprintf(flag.CommandLine.Output(), "Invalid index pattern: %v\n\n", *index)
			flag.Usage()
			os.Exit(1)
		}
	}
	if _, err := os.Stat(*root); err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Invalid root directory: %v\n\n", err)
		flag.Usage()
		os.Exit(1)
	}
	dir := os.DirFS(*root)

	// Startup the file server.
	var ln net.Listener
	for {
		var err error
		ln, err = net.Listen("tcp", *addr)
		if err == nil {
			break
		}
		const retryPeriod = 30 * time.Second
		log.Printf("net.Listen error: %v; retry in %v", err, retryPeriod)
		time.Sleep(retryPeriod)
	}
	log.Printf("started up server on %v", *addr)
	log.Fatal(http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if *verbose {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}

		// Verify that the file exists.
		f, err := dir.Open(filepath.Join(".", filepath.FromSlash(r.URL.Path)))
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
		if regexpMatch(denyRx, r.URL.Path) {
			httpError(w, r, os.ErrPermission)
			return
		}

		// Serve either a directory or a file.
		if fi.IsDir() {
			serveDirectory(w, r, dir, f)
		} else {
			serveFile(w, r, f, fi.ModTime(), true)
		}
	})))
}

func serveDirectory(w http.ResponseWriter, r *http.Request, dir fs.FS, f fs.File) {
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
	sort.Slice(fes, func(i, j int) bool {
		return fes[i].Name() < fes[j].Name()
	})

	type fileInfo struct {
		Name    string
		Size    int64
		ModTime time.Time
	}
	var fis []fileInfo
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
		if regexpMatch(hideRx, urlPath) || regexpMatch(denyRx, urlPath) {
			continue
		}
		if regexpMatch(indexRx, urlPath) {
			f, err := dir.Open(filepath.Join(".", filepath.FromSlash(r.URL.Path), fi.Name()))
			if err != nil {
				httpError(w, r, err)
				return
			}
			defer f.Close()
			r.URL.Path = urlPath
			serveFile(w, r, f, fi.ModTime(), false)
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
		fis = append(fis, fileInfo{Name: name, Size: size, ModTime: fi.ModTime()})
	}

	// Format the list of files and folders.
	renderHTML(w, r, func(w io.Writer) {
		io.WriteString(w, "<table>\n")
		io.WriteString(w, "<thead>\n")
		io.WriteString(w, "<tr>\n")
		io.WriteString(w, "<th>Name</th>\n")
		io.WriteString(w, "<th>Size</th>\n")
		io.WriteString(w, "<th>Last Modified</th>\n")
		io.WriteString(w, "</tr>\n")
		io.WriteString(w, "</thead>\n")
		io.WriteString(w, "<tbody>\n")
		now := time.Now()
		for _, fi := range fis {
			urlString := (&url.URL{Path: fi.Name}).String()
			io.WriteString(w, "<tr>\n")
			io.WriteString(w, "<td>")
			io.WriteString(w, `<a href="`+html.EscapeString(urlString)+`">`+html.EscapeString(fi.Name)+`</a>`)
			io.WriteString(w, "</td>\n")
			io.WriteString(w, "<td>")
			if !strings.HasSuffix(fi.Name, "/") {
				io.WriteString(w, html.EscapeString(formatSize(fi.Size)))
			}
			io.WriteString(w, "</td>\n")
			io.WriteString(w, "<td>")
			io.WriteString(w, html.EscapeString(formatTime(fi.ModTime, now)))
			io.WriteString(w, "</td>\n")
			io.WriteString(w, "</tr>\n")
		}
		io.WriteString(w, "</tbody>\n")
		io.WriteString(w, "</table>\n")
	})
}

func serveFile(w http.ResponseWriter, r *http.Request, f fs.File, modTime time.Time, allowRedirect bool) {
	if allowRedirect && regexpMatch(indexRx, r.URL.Path) {
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
	if !*sendfile {
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

// formatSize returns the formatted size with IEC prefixes.
// E.g., 81533654 => 77.8MiB
func formatSize(i int64) string {
	units := "=KMGTPEZY"
	n := float64(i)
	for n >= 1024 {
		n /= 1024
		units = units[1:]
	}
	if units[0] == '=' {
		return fmt.Sprintf("%dB", int(n))
	} else {
		return fmt.Sprintf("%0.1f%ciB", n, units[0])
	}
}

// formatTime formats the timestamp with second granularity.
// Timestamps within 12 hours of now only print the time (e.g., "3:04 PM"),
// otherwise it is formatted as only the date (e.g., "Jan 2, 2006").
func formatTime(ts, now time.Time) string {
	if d := ts.Sub(now); -12*time.Hour < d && d < 12*time.Hour {
		return ts.Format("3:04 PM")
	} else {
		return ts.Format("Jan 2, 2006")
	}
}

func renderHTML(w http.ResponseWriter, r *http.Request, renderBody func(io.Writer)) {
	var bb bytes.Buffer
	bb.WriteString("<html lang=\"en\">\n")
	bb.WriteString("<head>\n")
	bb.WriteString("<title>" + html.EscapeString(path.Base(r.URL.Path)) + "</title>\n")
	bb.WriteString("<style>\n")
	bb.WriteString("body { font-family: monospace; }\n")
	bb.WriteString("h1 { margin: 0; }\n")
	bb.WriteString("th, td { text-align: left; }\n")
	bb.WriteString("th, td { padding-right: 2em; }\n")
	bb.WriteString("th { padding-bottom: 0.5em; }\n")
	bb.WriteString("a, a:visited, a:hover, a:active { color: blue; }\n")
	bb.WriteString("</style>\n")
	bb.WriteString("</head>\n")
	bb.WriteString("<body>\n")

	// Format the title.
	bb.WriteString("<h1>")
	names := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	for i, name := range names {
		if i > 0 {
			bb.WriteString(" ")
		}
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
		bb.WriteString(`<a href="` + html.EscapeString(urlString) + `">` + html.EscapeString(name) + `</a>`)
	}
	bb.WriteString("</h1>\n")
	bb.WriteString("<hr>\n")

	renderBody(&bb)

	bb.WriteString("</body>\n")
	bb.WriteString("</html>\n")

	w.Write(bb.Bytes())
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
	renderHTML(w, r, func(w io.Writer) {
		io.WriteString(w, http.StatusText(code)+": "+html.EscapeString(err.Error()))
	})
}
