package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/vansh845/smoltorrent/internal/decoder"
	"github.com/vansh845/smoltorrent/internal/peer"
	"github.com/vansh845/smoltorrent/internal/torrent"
)

type PeerMessage int

const (
	CHOKE = iota
	UNCHOKE
	INTERESTED
	NOT_INTERESTED
	HAVE
	BITFIELD
	REQUEST
	PIECE
	CANCEL
)

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	// fmt.Println("Logs from your program will appear here!")

	command := os.Args[1]

	switch command {
	case "decode":
		bencodedValue := os.Args[2]

		decoded, err := decoder.DecodeBencode(strings.NewReader(bencodedValue))
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	case "info":

		torrentFile := os.Args[2]

		torrent, err := torrent.NewTorrent(torrentFile)

		if err != nil {
			panic(err)
		}
		infoHash, err := torrent.InfoHash()
		fmt.Printf("Tracker URL: %s\n", torrent.Announce)
		fmt.Printf("Length: %d\n", torrent.Info.Length)
		fmt.Printf("Info Hash: %x\n", infoHash)
		fmt.Printf("Piece Length: %d\n", torrent.Info.Piece.Length)
		fmt.Printf("Piece Hashes: %x\n", torrent.Info.Piece.Hashes)
	case "peers":

		torrentFile := os.Args[2]
		torrent, err := torrent.NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}
		peers, err := torrent.DiscoverPeers()
		if err != nil {
			panic(err)
		}

		for _, x := range peers {
			fmt.Println(x.ToString())
		}
	case "handshake":

		ipaddr := os.Args[3]
		torrentFile := os.Args[2]
		torrent, err := torrent.NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}

		peer := peer.NewPeerFromString(ipaddr)
		infoHash, err := torrent.InfoHash()
		if err != nil {
			panic(err)
		}
		err = peer.Connect()
		if err != nil {
			panic(err)
		}
		buff := peer.SendHandshake(infoHash)

		fmt.Printf("Peer ID: %s\n", hex.EncodeToString(buff[48:]))
	case "download_piece":

		torrentFile := os.Args[4]
		index, _ := strconv.Atoi(os.Args[5])
		torrent.HandleDownloadPiece(torrentFile, index)
	case "download":

		torrentFile := os.Args[4]
		torrent.HandleDownloadFile(torrentFile)

	default:
		fmt.Println("command not found")
	}
}
