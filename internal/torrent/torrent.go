package torrent

import (
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"

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

func Downloaded(torrent *Torrent, peer peer.Peer, idx int, infoHash []byte) (bool, error) {

	res, err := peer.SendHandshake(infoHash)
	if err != nil {
		return false, err
	}
	fmt.Println(string(res))
	//wait for bitfield
	res, err = peer.WaitForMessage(BITFIELD)
	if err != nil {
		return false, err
	}
	fmt.Println(string(res))
	bitfield := []byte{}
	for _, x := range res {
		binRep := fmt.Sprintf("%08b", x)
		bitfield = append(bitfield, []byte(binRep)...)
	}
	fmt.Println(bitfield)
	if !peer.HasPiece(idx, string(bitfield)) {
		return false, err
	}

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

	length := torrent.Info.Piece.Length
	if idx == torrent.Info.Piece.Count-1 {
		length = torrent.Info.Length % torrent.Info.Piece.Length
	}
	fmt.Println("starting download of piece", idx+1)
	peer.DownloadPiece([]byte(torrent.Info.Piece.Hashes), length, idx)

	return true, nil
}

func spawnCr(torrent *Torrent, wg *sync.WaitGroup, peerCh chan peer.Peer, i int, infoHash []byte) {
	defer wg.Done()

	peer := <-peerCh
	err := peer.Connect()
	if err != nil {
		return
	}
	fmt.Println(peer)
	ok, err := Downloaded(torrent, peer, i, infoHash)
	peerCh <- peer
	if !ok {
		fmt.Printf("%d piece failed, trying again\n", i)
		spawnCr(torrent, wg, peerCh, i, infoHash)
	}
}
func HandleDownloadFile(torrentFile string) {

	torrent, err := NewTorrent(torrentFile)
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
	peerCh := make(chan peer.Peer, len(peers))
	for _, peer := range peers {
		peerCh <- peer
	}
	fmt.Println(peers)
	if err != nil {
		panic(err)
	}
	fmt.Println(torrent.Info.Piece.Count)
	for i := 0; i < torrent.Info.Piece.Count; i++ {
		wg.Add(1)
		go spawnCr(torrent, &wg, peerCh, i, infoHash)
	}
	wg.Wait()
	files, err := os.ReadDir("pieces")
	if err != nil {
		panic(err)
	}
	finalPiece, _ := os.OpenFile(torrent.Info.Name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	for _, x := range files {
		p, err := os.Open(fmt.Sprintf("pieces/%s", x.Name()))
		if err != nil {
			panic(err)
		}
		_, err = io.Copy(finalPiece, p)
		if err != nil {
			panic(err)
		}
		err = os.Remove(fmt.Sprintf("pieces/%s", x.Name()))

		if err != nil {
			panic(err)
		}
	}
	fmt.Println("File downloaded...")

}

func GeneratePeerId() []byte {
	peerId := make([]byte, 20)
	rand.Read(peerId)
	return peerId
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
	length := -1
	for k := range inDict {
		if k == "pieces" {
			continue
		}
		files, ok := inDict[k].([]interface{})
		if ok {
			for _, file := range files {
				mpFile := file.(map[string]interface{})
				for k, v := range mpFile {
					if k == "length" {
						length = v.(int)
					}
				}

			}
		}
	}
	pl := inDict["piece length"].(int)
	pieceHash := inDict["pieces"].(string)
	count := len(pieceHash) / 20

	var piece Piece = Piece{
		Length: pl,
		Hashes: pieceHash,
		Count:  count,
	}

	if length == -1 {
		length = inDict["length"].(int)
	}
	var info Info = Info{
		Length: length,
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
