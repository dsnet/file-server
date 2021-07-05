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
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// TODO: Add basic authentication.

var (
	addr    = flag.String("addr", ":8080", "The network address to listen on.")
	hide    = flag.String("hide", "/[.]", "Regular expression of file paths to hide. Paths matching this pattern are excluded from directory listings, but direct fetches for this path are still resolved.")
	exclude = flag.String("exclude", "", "Regular expression of file paths to exclude. Paths matching this pattern are excluded from directory listings and direct fetches for this path report NotFound.")
	index   = flag.String("index", "", "Name of the index page to directly render for a directory. (e.g., 'index.html'; default none)")
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

		// Serve either a directory or a file.
		if fi.IsDir() {
			serveDirectory(w, r, fp)
		} else {
			http.ServeFile(w, r, fp)
		}
	})))
}

func serveDirectory(w http.ResponseWriter, r *http.Request, fp string) {
	// Serve the index page directly (if possible).
	if *index != "" {
		fp2 := filepath.Join(fp, *index)
		_, err := os.Stat(fp2)
		if err == nil {
			http.ServeFile(w, r, fp2)
			return
		} else if !os.IsNotExist(err) {
			httpError(w, err)
			return
		}
	}

	// Read the directory entries.
	fis, err := os.ReadDir(fp)
	if err != nil {
		httpError(w, err)
		return
	}

	// Format the header.
	var bb bytes.Buffer
	bb.WriteString("<html>\n")
	bb.WriteString("<head>\n")
	bb.WriteString("<title>" + html.EscapeString(r.URL.Path) + "</title>\n")
	bb.WriteString("<style>\n")
	bb.WriteString("body { font-family: mono; }\n")
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
			bb.WriteString(`<a href="` + html.EscapeString(urlPath) + `">` + html.EscapeString(name) + `</a>`)
			prevIdx = currIdx
		}
	}
	bb.WriteString("</h1>\n")
	bb.WriteString("<hr>\n")

	// Format the list of files and folders.
	bb.WriteString("<ul>\n")
	for _, fi := range fis {
		name := fi.Name()
		urlPath := path.Join(r.URL.Path, name)
		if regexpMatch(hideRx, urlPath) || regexpMatch(excludeRx, urlPath) {
			continue
		}
		if fi.IsDir() {
			name += "/"
		}
		bb.WriteString("<li>")
		bb.WriteString(`<a href="` + html.EscapeString(urlPath) + `">` + html.EscapeString(name) + `</a>`)
		bb.WriteString("</li>\n")
	}
	bb.WriteString("</ul>\n")

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
