# wiki-api
`TODO: find a better name.`

This is a prototype backend to a new wiki built for flipdot.

Basically, it uses libgit2 to expose a git repository as a simple web-server.
All template rendering (except for "Index of") should be done in the separate front-end.

## Usage

Just `go get` and point it to the repository you want to serve:

```bash
go get -u github.com/cfstras/wiki-api
wiki-api ~/path-to/wiki-data.git
```

For development:
```bash
cd $GOPATH/github.com/cfstras/wiki-api
<edit some files>
go generate ./...  # regenerate asset files
go get -v && wiki-api ~/path-to/wiki-data.git
```

## API
The API is planned as follows:

- `GET /`  |  `GET /folder/subfolder/`  
Returns an index-of listing, rendered in HTTPD style.

- `GET /file.md`  |  `GET /folder/file.md`  
Returns the file content.

- `GET /file.md.json`  |  `GET /folder.json` | `GET /.json`  
Returns file/folder information rendered as JSON, along with history entries.

- `GET /file.md.history/`  |  `GET /folder.history/`  
Returns index-of listing of file/folder history.

- `GET /file.md.history/[12-]954abcf2` / `GET /folder.history/[12-]954abcf2/`  
Returns file/folder contents at commit-id. The number in front is used for sorting and
can be omitted.

`TODO: PUT, DELETE, auth`

# License
GPLv2.
