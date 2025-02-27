package main

import (
	"os"

	"github.com/vansh845/smoltorrent/internal/torrent"
)

func main() {

	torrentFile := os.Args[1]
    torrent.HandleDownloadFile(torrentFile)

}
