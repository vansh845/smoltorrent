package torrent

import (
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/jackpal/bencode-go"
	"github.com/vansh845/smoltorrent/internal/decoder"
  "github.com/vansh845/smoltorrent/internal/peer"
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

func GeneratePeerId() []byte {
	peerId := make([]byte, 20)
	rand.Read(peerId)
	return peerId
}


type Torrent struct {
	Announce string
	Info     Info
	mp       map[string]interface{}
}

type Info struct {
	Length int
	Name   string
	Piece  Piece
}

type Piece struct {
	Length int
	Hashes string
	Count  int
}

func (tr *Torrent) DiscoverPeers() ([]peer.Peer, error) {

	infoHash, err := tr.InfoHash()
	peerId := GeneratePeerId()
	port := "6881"
	left := fmt.Sprintf("%d", tr.Info.Length)

	params := url.Values{}
	params.Add("info_hash", string(infoHash))
	params.Add("peer_id", string(peerId))
	params.Add("port", port)
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", left)
	params.Add("compact", "1")

	url := tr.Announce + "?" + params.Encode()
	//send request to announce
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	decoded, err := decoder.DecodeBencode(resp.Body)
	if err != nil {
		return nil, err
	}
	respMap := decoded.(map[string]interface{})
	ps := respMap["peers"].(string)
	peerByte := []byte(ps)
	peers := make([]peer.Peer, 0)
	for i := 0; i < len(peerByte); i += 6 {

		peer := peer.New(peerByte[i : i+6])
		if peer.IpAddr.String() == "0.0.0.0" {
			continue
		}
		peers = append(peers, peer)

	}
	return peers, nil
}

func (tr *Torrent) InfoHash() ([]byte, error) {

	h := sha1.New()
	err := bencode.Marshal(h, tr.mp)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func NewTorrent(torrentFile string) (*Torrent, error) {

	file, err := os.Open(torrentFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := decoder.NewDecoder(file)
	output, err := decoder.Decode()
	if err != nil {
		return nil, err
	}

	dict, ok := output.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s is not a bencoded dictionary\n", file.Name())
	}
	inDict, ok := dict["info"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no info section in %s\n", file.Name())
	}

	pl := inDict["piece length"].(int)
	pieceHash := inDict["pieces"].(string)
	count := len(pieceHash) / 20

	var piece Piece = Piece{
		Length: pl,
		Hashes: pieceHash,
		Count:  count,
	}

	l := inDict["length"].(int)
	var info Info = Info{
		Length: l,
		Name:   inDict["name"].(string),
		Piece:  piece,
	}

	torrent := &Torrent{
		Announce: dict["announce"].(string),
		Info:     info,
		mp:       inDict,
	}

	return torrent, nil
}
