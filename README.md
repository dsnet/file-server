# File Server

This program is a simple HTTP file server.

## Usage

The server can be installed with:
```
$ go install github.com/dsnet/file-server@latest
```

The server is started up by running:
```
$ file-server
```

By default, the server starts up listening on `:8080` and
serves files from the current working directory.

For more options, see `filer-server -help`:
```
Usage: ./file-server [OPTION]...

  -addr string
        The network address to listen on. (default ":8080")
  -exclude string
        Regular expression of file paths to exclude.
        Paths matching this pattern are excluded from directory listings
        and direct fetches for this path report NotFound.
  -hide string
        Regular expression of file paths to hide.
        Paths matching this pattern are excluded from directory listings, 
        but direct fetches for this path are still resolved. (default "/[.]")
  -index string
        Name of the index page to directly render for a directory.
        (e.g., 'index.html'; default none)
  -root string
        Directory to serve files from. (default ".")
  -verbose
        Log every HTTP request.
```