package main

import (
	"fmt"
	"os"

	"github.com/vansh845/smoltorrent/internal/torrent"
)

func main() {

	torrentFile := os.Args[1]
    torrent , err := torrent.NewTorrent(torrentFile)
    if err != nil{
        panic(err)
    }
    fmt.Println(torrent.Announce)

    peers , err := torrent.DiscoverPeers()
    if err != nil{
        panic(err)
    }
    fmt.Println(peers)

}
