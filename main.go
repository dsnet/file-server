// Copyright 2021, Joe Tsai. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"time"
)

func main() {
	var (
		addr     = flag.String("addr", ":8080", "The network address to listen on.")
		hide     = flag.String("hide", "/[.][^/]+/?$", "Regular expression of file paths to hide.\nPaths matching this pattern are excluded from directory listings,\nbut direct requests for this path are still resolved.")
		deny     = flag.String("deny", "", "Regular expression of file paths to deny.\nPaths matching this pattern are excluded from directory listings\nand direct requests for this path report StatusForbidden.")
		index    = flag.String("index", "", "Regular expression of file paths to treat as index.html pages.\n(e.g., '/index[.]html$'; default none)")
		root     = flag.String("root", ".", "Directory to serve files from.")
		sendfile = flag.Bool("sendfile", true, "Allow the use of the sendfile syscall.")
		verbose  = flag.Bool("verbose", false, "Log every HTTP request.")
	)

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
	var srv Server
	if *hide != "" {
		srv.hideRx, err = regexp.Compile(*hide)
		if err != nil {
			fmt.Fprintf(flag.CommandLine.Output(), "Invalid hide pattern: %v\n\n", *hide)
			flag.Usage()
			os.Exit(1)
		}
	}
	if *deny != "" {
		srv.denyRx, err = regexp.Compile(*deny)
		if err != nil {
			fmt.Fprintf(flag.CommandLine.Output(), "Invalid deny pattern: %v\n\n", *deny)
			flag.Usage()
			os.Exit(1)
		}
	}
	if *index != "" {
		srv.indexRx, err = regexp.Compile(*index)
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
	srv.root = os.DirFS(*root)
	srv.sendfile = *sendfile
	srv.verbose = *verbose

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
	log.Fatal(http.Serve(ln, http.HandlerFunc(srv.ServeHTTP)))
}
