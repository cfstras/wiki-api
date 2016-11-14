# wiki-api [![Build Status](https://travis-ci.org/cfstras/wiki-api.svg?branch=master)](https://travis-ci.org/cfstras/wiki-api)
`TODO: find a better name.`

This is a prototype backend to a new wiki built for flipdot.

Basically, it uses libgit2 to expose a git repository as a simple web-server.
All template rendering (except for "Index of") should be done in the separate front-end.

## Usage

[Download binary here](https://github.com/cfstras/wiki-api/releases). Then start it, and point at your data repository.
```bash
./wiki-api ~/path-to/wiki-data.git

# help:
./wiki-api ~/path-to/wiki-data.git --help
```

### For development:
```bash
go get github.com/cfstras/wiki-api
cd $GOPATH/github.com/cfstras/wiki-api && go generate -v ./...  # regenerate asset files if you changed them
go get -v && wiki-api -debug ~/path-to/wiki-data.git
```

## API
The API is as follows:

### `GET /`  |  `GET /folder/subfolder/`  
Returns an index-of listing, rendered in HTTPD style.

### `GET /file.md`  |  `GET /folder/file.md`  
Returns the file content.

### `GET /file.md.json`  |  `GET /folder/.json` | `GET /.json`  
Returns file/folder information rendered as JSON, along with history entries.

### _not implemented_ `GET /file.md.history/`  |  `GET /folder.history/`  
Returns index-of listing of file/folder history.

### _not implemented_ `GET /file.md.history/[12-]954abcf2` / `GET /folder.history/[12-]954abcf2/`  
Returns file/folder contents at commit-id. The number in front is used for sorting and
can be omitted.

### `PUT /file.md` | `PUT /foo/file.md`
Creates or updates a file. The directory does not have to exist, and will be created on-the-fly if necessary.  
The body of the request will be used verbatim as the file contents.

Additional headers:  

- `Auth: token`: Token used for authorization
- `Wiki-Last-Id: <sha256>` (optional): the sha256 of the object to be replaced.
  Can be used to verify that the file was not updated by somebody else.  
  Set to `null` to ensure the file does not exist before creating it.
- `Wiki-Commit-Msg` (optional): Set a commit message describing the changes.

Responds with the Commit ID of the newly generated commit, or an error message.

Response codes:

- 200 OK: everything was okay!
- 409 Conflict: the `Last-Id` header did not match. Please re-fetch file information and merge changes.  
    Also occurs on other conflicts, e.g. creating a file ending in `.json`.
- 410 Gone: a `Last-Id` header was supplied, but the file did not exist before.


`TODO: DELETE, auth`

# License
GPLv2.
