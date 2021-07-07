// Copyright 2021, Joe Tsai. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"html"
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

// TODO: Add basic authentication.

var (
	addr    = flag.String("addr", ":8080", "The network address to listen on.")
	hide    = flag.String("hide", "/[.][^/]+$", "Regular expression of file paths to hide.\nPaths matching this pattern are excluded from directory listings,\nbut direct fetches for this path are still resolved.")
	exclude = flag.String("exclude", "", "Regular expression of file paths to exclude.\nPaths matching this pattern are excluded from directory listings\nand direct fetches for this path report NotFound.")
	index   = flag.String("index", "", "Name of the index page to directly render for a directory.\n(e.g., 'index.html'; default none)")
	root    = flag.String("root", ".", "Directory to serve files from.")
	verbose = flag.Bool("verbose", false, "Log every HTTP request.")

	hideRx    *regexp.Regexp
	excludeRx *regexp.Regexp
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
	if *exclude != "" {
		excludeRx, err = regexp.Compile(*exclude)
		if err != nil {
			fmt.Fprintf(flag.CommandLine.Output(), "Invalid exclude pattern: %v\n\n", *exclude)
			flag.Usage()
			os.Exit(1)
		}
	}
	if strings.Contains(*index, "/") || *index == "." || *index == ".." {
		fmt.Fprintf(flag.CommandLine.Output(), "Invalid index name: %v\n\n", *index)
		flag.Usage()
		os.Exit(1)
	}
	if _, err := os.Stat(*root); err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Invalid root directory: %v\n\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// Startup the file server.
	log.Printf("starting up server on %v", *addr)
	log.Fatal(http.ListenAndServe(*addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never cache the server results. Consider it dynamically changing.
		w.Header().Set("Cache-Control", "no-cache, no-store, no-transform, must-revalidate, private, max-age=0")

		// For simplicity, always deal with clean paths that are absolute.
		r.URL.Path = "/" + strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		// Log the request.
		if *verbose {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}

		// Exclude paths that match the exclude pattern.
		if regexpMatch(excludeRx, r.URL.Path) {
			httpError(w, os.ErrNotExist)
			return
		}

		// Verify that the file exists.
		fp := filepath.Join(*root, filepath.FromSlash(r.URL.Path))
		fi, err := os.Stat(fp)
		if err != nil {
			httpError(w, err)
			return
		}
		f, err := os.Open(fp)
		if err != nil {
			httpError(w, err)
			return
		}
		defer f.Close()

		// Serve either a directory or a file.
		if fi.IsDir() {
			serveDirectory(w, r, fp, f)
		} else {
			http.ServeContent(w, r, fp, fi.ModTime(), f)
		}
	})))
}

func serveDirectory(w http.ResponseWriter, r *http.Request, fp string, f *os.File) {
	// Serve the index page directly (if possible).
	if *index != "" {
		fp2 := filepath.Join(fp, *index)
		fi2, err := os.Stat(fp2)
		if err == nil {
			f2, err := os.Open(fp2)
			if err != nil {
				httpError(w, err)
				return
			}
			defer f2.Close()
			http.ServeContent(w, r, fp2, fi2.ModTime(), f2)
			return
		} else if !os.IsNotExist(err) {
			httpError(w, err)
			return
		}
	}

	// Read the directory entries, resolving any symbolic links,
	// and sorting all the entries by name.
	fis, err := f.Readdir(0)
	if err != nil {
		httpError(w, err)
		return
	}
	for i, fi := range fis {
		if fi.Mode()*os.ModeSymlink > 0 {
			if fi, _ := os.Stat(filepath.Join(fp, fi.Name())); fi != nil {
				fis[i] = fi // best effort resolution
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
	bb.WriteString("</style>\n")
	bb.WriteString("</head>\n")
	bb.WriteString("<body>\n")

	// Format the title.
	bb.WriteString("<h1>")
	r.URL.Path = strings.TrimSuffix(r.URL.Path, "/") + "/"
	var prevIdx int
	for i, c := range r.URL.Path {
		if c == '/' {
			currIdx := i + len("/")
			name := r.URL.Path[prevIdx:currIdx]
			urlPath := r.URL.Path[:currIdx]
			if prevIdx != 0 {
				urlPath = strings.TrimSuffix(urlPath, "/")
				bb.WriteString(" ")
			}
			urlString := (&url.URL{Path: urlPath}).String()
			bb.WriteString(`<a href="` + html.EscapeString(urlString) + `">` + html.EscapeString(name) + `</a>`)
			prevIdx = currIdx
		}
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
	for _, fi := range fis {
		name := fi.Name()
		urlPath := path.Join(r.URL.Path, name)
		urlString := (&url.URL{Path: urlPath}).String()
		if regexpMatch(hideRx, urlPath) || regexpMatch(excludeRx, urlPath) {
			continue
		}
		if fi.IsDir() {
			name += "/"
		}
		bb.WriteString("<tr>\n")
		bb.WriteString("<td>")
		bb.WriteString(`<a href="` + html.EscapeString(urlString) + `">` + html.EscapeString(name) + `</a>`)
		bb.WriteString("</td>\n")
		bb.WriteString("<td>")
		if fi.Mode().IsRegular() {
			bb.WriteString(formatSize(fi.Size()))
		}
		bb.WriteString("</td>\n")
		bb.WriteString("<td>")
		bb.WriteString(fi.ModTime().Round(time.Second).UTC().Format("2006-01-02 15:04:05"))
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
