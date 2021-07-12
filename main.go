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
	hide     = flag.String("hide", "/[.][^/]+/?$", "Regular expression of file paths to hide.\nPaths matching this pattern are excluded from directory listings,\nbut direct fetches for this path are still resolved.")
	deny     = flag.String("deny", "", "Regular expression of file paths to deny.\nPaths matching this pattern are excluded from directory listings\nand direct fetches for this path report StatusForbidden.")
	index    = flag.String("index", "", "Name of the index page to directly render for a directory.\n(e.g., 'index.html'; default none)")
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
	log.Printf("starting up server on %v", *addr)
	log.Fatal(http.ListenAndServe(*addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			httpError(w, err)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			httpError(w, err)
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
			httpError(w, os.ErrPermission)
			return
		}

		// Serve either a directory or a file.
		if fi.IsDir() {
			serveDirectory(w, r, dir, f)
		} else {
			serveFile(w, r, f, fi, true)
		}
	})))
}

func serveDirectory(w http.ResponseWriter, r *http.Request, dir fs.FS, f fs.File) {
	// Read the directory entries, resolving any symbolic links,
	// and sorting all the entries by name.
	fd, ok := f.(fs.ReadDirFile)
	if !ok {
		httpError(w, os.ErrInvalid)
		return
	}
	fes, err := fd.ReadDir(0)
	if err != nil {
		httpError(w, err)
		return
	}
	fis := make([]fs.FileInfo, 0, len(fes))
	for _, fe := range fes {
		if fe.Type()&os.ModeSymlink == 0 {
			if fi, _ := fe.Info(); fi != nil {
				fis = append(fis, fi)
			}
		} else {
			if fi, _ := fs.Stat(dir, filepath.Join(".", filepath.FromSlash(r.URL.Path), fe.Name())); fi != nil {
				fis = append(fis, fi) // best effort symlink resolution
			}
		}
	}
	sort.Slice(fis, func(i, j int) bool {
		return fis[i].Name() < fis[j].Name()
	})

	// Format the header.
	var bb bytes.Buffer
	bb.WriteString("<html lang=\"en\">\n")
	bb.WriteString("<head>\n")
	bb.WriteString("<title>" + html.EscapeString(r.URL.Path) + "</title>\n")
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
		urlString := "." + strings.Repeat("/..", len(names)-1-i)
		bb.WriteString(`<a href="` + html.EscapeString(urlString) + `">` + html.EscapeString(name+"/") + `</a>`)
	}
	bb.WriteString("</h1>\n")

	bb.WriteString("<hr>\n")

	// Format the list of files and folders.
	bb.WriteString("<table>\n")
	bb.WriteString("<thead>\n")
	bb.WriteString("<tr>\n")
	bb.WriteString("<th>Name</th>\n")
	bb.WriteString("<th>Size</th>\n")
	bb.WriteString("<th>Last Modified</th>\n")
	bb.WriteString("</tr>\n")
	bb.WriteString("</thead>\n")
	bb.WriteString("<tbody>\n")
	now := time.Now()
	for _, fi := range fis {
		name := fi.Name()
		if fi.IsDir() {
			name += "/"
		}
		urlPath := r.URL.Path + "/" + name
		urlString := (&url.URL{Path: name}).String()
		if regexpMatch(hideRx, urlPath) || regexpMatch(denyRx, urlPath) {
			continue
		}
		if regexpMatch(indexRx, urlPath) {
			f, err := dir.Open(filepath.Join(".", filepath.FromSlash(r.URL.Path), fi.Name()))
			if err != nil {
				httpError(w, err)
				return
			}
			defer f.Close()
			r.URL.Path = urlPath
			serveFile(w, r, f, fi, false)
			return
		}
		bb.WriteString("<tr>\n")
		bb.WriteString("<td>")
		bb.WriteString(`<a href="` + html.EscapeString(urlString) + `">` + html.EscapeString(name) + `</a>`)
		bb.WriteString("</td>\n")
		bb.WriteString("<td>")
		if fi.Mode().IsRegular() {
			bb.WriteString(html.EscapeString(formatSize(fi.Size())))
		}
		bb.WriteString("</td>\n")
		bb.WriteString("<td>")
		bb.WriteString(html.EscapeString(formatTime(fi.ModTime(), now)))
		bb.WriteString("</td>\n")
		bb.WriteString("</tr>\n")
	}
	bb.WriteString("</tbody>\n")
	bb.WriteString("</table>\n")

	// Format the footer.
	bb.WriteString("</body>\n")
	bb.WriteString("</html>\n")
	w.Write(bb.Bytes())
}

func serveFile(w http.ResponseWriter, r *http.Request, f fs.File, fi fs.FileInfo, allowRedirect bool) {
	if allowRedirect && regexpMatch(indexRx, r.URL.Path) {
		relativeRedirect(w, r, "./") // redirect to directory containing index.html
		return
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		b, err := io.ReadAll(f)
		if err != nil {
			httpError(w, err)
			return
		}
		rs = bytes.NewReader(b)
	}
	if !*sendfile {
		rs = struct{ io.ReadSeeker }{rs} // drop ReadFrom method to avoid using sendfile syscall
	}
	http.ServeContent(w, r, r.URL.Path, fi.ModTime(), rs)
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

func httpError(w http.ResponseWriter, err error) {
	switch {
	case os.IsNotExist(err):
		http.Error(w, "404 page not found", http.StatusNotFound)
	case os.IsPermission(err):
		http.Error(w, "403 Forbidden", http.StatusForbidden)
	default:
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
	}
}
