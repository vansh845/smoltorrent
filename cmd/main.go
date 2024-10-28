package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

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
		torrent, err := torrent.NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}
		peers, err := torrent.DiscoverPeers()
		if err != nil {
			panic(err)
		}
		infoHash, err := torrent.InfoHash()
		if err != nil {
			panic(err)
		}
		peer := peers[0]

		//make a tcp connection with peer
		err = peer.Connect()
		if err != nil {
			panic(err)
		}
		//send handshake
		buff := peer.SendHandshake(infoHash)
		fmt.Println(buff)

		//wait for bitfield
		res, err := peer.WaitForMessage(BITFIELD)
		if err != nil {
			panic(err)
		}

		fmt.Println(res)
		// send intereseted
		err = peer.SendMessage(INTERESTED, nil)
		if err != nil {
			panic(err)
		}

		// wait for unchoke
		res, err = peer.WaitForMessage(UNCHOKE)
		if err != nil {
			panic(err)
		}

		fmt.Println(res)
		// send request message

		infoHash, err = torrent.InfoHash()
		if err != nil {
			panic(err)
		}
		length := torrent.Info.Piece.Length
		if index == torrent.Info.Piece.Count-1 {
			length = torrent.Info.Length % torrent.Info.Piece.Length
		}
		piece := peer.DownloadPiece([]byte(torrent.Info.Piece.Hashes), length, index)

		filePath := os.Args[3]
		fd, err := os.Create(filePath)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		_, err = fd.Write(piece)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "download":

		torrentFile := os.Args[4]

		torrent, err := torrent.NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}

		infoHash, err := torrent.InfoHash()
		if err != nil {
			panic(err)
		}
		err = os.Mkdir("pieces", os.ModePerm)
		if err != nil {
			if !os.IsExist(err) {
				panic(err)
			}
		}

		wg := sync.WaitGroup{}
		peers, err := torrent.DiscoverPeers()
		if err != nil {
			panic(err)
		}
		fmt.Println(peers)
		for i := 0; i < torrent.Info.Piece.Count; i++ {
			wg.Add(1)
			go func(wg *sync.WaitGroup) {
				defer wg.Done()

				peer := peers[i%len(peers)]
				err = peer.Connect()
				if err != nil {
					panic(err)
				}
				//send handshake
				buff := peer.SendHandshake(infoHash)
				fmt.Println(string(buff))

				//wait for bitfield
				res, err := peer.WaitForMessage(BITFIELD)
				if err != nil {
					panic(err)
				}

				fmt.Println(res)
				// send intereseted
				err = peer.SendMessage(INTERESTED, nil)
				if err != nil {
					panic(err)
				}

				fmt.Println(res)
				// wait for unchoke
				res, err = peer.WaitForMessage(UNCHOKE)
				if err != nil {
					panic(err)
				}

				fmt.Println(res)
				length := torrent.Info.Piece.Length
				if i == torrent.Info.Piece.Count-1 {
					length = torrent.Info.Length % torrent.Info.Piece.Length
				}
				peer.DownloadPiece([]byte(torrent.Info.Piece.Hashes), length, i)
			}(&wg)
		}
		wg.Wait()
		files, err := os.ReadDir("pieces")
		if err != nil {
			panic(err)
		}
		finalPiece, _ := os.OpenFile(os.Args[3], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		for _, x := range files {
			p, err := os.Open(fmt.Sprintf("pieces/%s", x.Name()))
			if err != nil {
				panic(err)
			}
			_, err = io.Copy(finalPiece, p)
			if err != nil {
				panic(err)
			}

		}
	default:
		fmt.Println("command not found")
	}
}
