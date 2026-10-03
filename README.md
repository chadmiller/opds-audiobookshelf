
A typical run of this software will look something like this.

```bash
go run github.com/chadmiller/opds-audiobookshelf/cmd/server@main \
  --addr :8080 \
  --scan-interval 3m \
  --title chadbooks \
  --libraries ebooks \
  --db /etc/audiobookshelf/config/absdatabase.sqlite \
  --files-root /data/media/ebooks
```

The addr flag sets the listen address for the web server. The title flag is
the name of the server as advertized inside the OPDS protocol.

The libraries flag is a comma-seperated list of libraries as known to audiobookshelf, that you want to make available to the network.

The db is the file path of the audiobookshelf absdatabase Sqlite3 file. This is
file is made and maintained by audiobookshelf.

The files-root is the location on disk of the files listed inside audiobookshelf.

A docker-compose stanza would look like

```yaml
...

  opds-audiobookshelf-ebooks:
    image: golang:1
    hostname: opds-ebooks
    restart: unless-stopped
    volumes:
      - /etc/audiobookshelf/config:/config:ro
      - /data/media/ebooks:/ebooks:ro
    entrypoint: ["/usr/local/go/bin/go", "run", "github.com/chadmiller/opds-audiobookshelf/cmd/server@maih", "--addr", ":8080", "--scan-interval", "3m", "--db", "/config/absdatabase.sqlite", "--files-root", "/ebooks", "--title", "chadbooks", "--libraries", "ebooks" ]
```
