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

For more options, see `file-server -help`:
```
Usage: ./file-server [OPTION]...

  -addr string
    	The network address to listen on. (default ":8080")
  -deny string
    	Regular expression of file paths to deny.
    	Paths matching this pattern are excluded from directory listings
    	and direct requests for this path report StatusForbidden.
  -hide string
    	Regular expression of file paths to hide.
    	Paths matching this pattern are excluded from directory listings,
    	but direct requests for this path are still resolved. (default "/[.][^/]+/?$")
  -index string
    	Regular expression of file paths to treat as index.html pages.
    	(e.g., '/index[.]html$'; default none)
  -root string
    	Directory to serve files from. (default ".")
  -sendfile
    	Allow the use of the sendfile syscall. (default true)
  -verbose
    	Log every HTTP request.
```
