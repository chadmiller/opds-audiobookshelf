 The problem, and solution

# You have ebooks

Specifically, you have an instance of
[audiobookshelf](https://audiobookshelf.org/) and you have books (ebooks
probably) in it.

# you have a reader device that speaks OPDS

You have a device that understands
[Open Publication Distribution System, OPDS](https://opds.io/) (version 1 or 2),
like one that is running [CrossPoint Reader](https://crosspointreader.com/) or
[CrossInk](https://crossink.dev/).
These are often devices like the Xteink X3, Xteink X4 Classic, Xteink X4 Pro,
Book Picco, M5PaperMono, M5Paper, Seeed Sticky.

# putting books on your ebook device is a hassle

The device is the worst place to maintain your list. It should be a
get-and-read tool.

You could pre-load everything on your device, but maintaining the books on
device is hard. Or devices you don't control should have access to your
library.

# solution, host this software

Run this to connect up your OPDS-speaking book device and your library.

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

The libraries flag is a comma-seperated list of libraries as known to
audiobookshelf, that you want to make available to the network.

The db is the file path of the audiobookshelf absdatabase Sqlite3 file. This is
file is made and maintained by audiobookshelf. (Mucking with the internals of ABS
is probably a terrible idea from a maintenance standpoint. FIXME.)

The files-root is the location on disk of the files listed inside audiobookshelf.

## run it

```bash
go run github.com/chadmiller/opds-audiobookshelf/cmd/server@main \
  --addr :8080 \
  --scan-interval 3m \
  --title chadbooks \
  --libraries ebooks \
  --db /etc/audiobookshelf/config/absdatabase.sqlite \
  --files-root /data/media/ebooks
```

## docker-compose stanza

Running it from a shell like above is good to try it, but to add it to your life,
you need something better. Just load up the docker image for the Go toolset and
ask it to run this tool. Put this stanza in a
[docker-compose](https://docs.docker.com/compose/) file, probably alongside
your instance of audiobookshelf.

```yaml
...

  opds-audiobookshelf-ebooks:
    image: golang:1
    hostname: opds-ebooks
    restart: unless-stopped
    volumes:
      - /etc/audiobookshelf/config:/config:ro
      - /data/media/ebooks:/ebooks:ro
    entrypoint: ["/usr/local/go/bin/go", "run", "github.com/chadmiller/opds-audiobookshelf/cmd/server@main", "--addr", ":8080", "--scan-interval", "3m", "--db", "/config/absdatabase.sqlite", "--files-root", "/ebooks", "--title", "chadbooks", "--libraries", "ebooks" ]
```
