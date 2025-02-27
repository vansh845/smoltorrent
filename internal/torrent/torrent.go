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

type TrackerResponse struct{
    Interval int
    Peers string
    Failure string
}

type TrackerPeer struct{
    PeerId string `bencode:"peer id"`
    Ip string `bencode:"ip"`
    Port string `bencode:"port"`
}

type Torrent struct{
    Announce string `bencode:"announce"`
    Info Info `bencode:"info"`
}
type Info struct{
    Files []File `bencode:"files"`
    Length int  `bencode:"length"`
    Name string `bencode:"name"`
    PieceLength int  `bencode:"piece length"`
    Pieces string `bencode:"pieces"`
}

type File struct{
    Length int `bencode:"length"`
    Path []string `bencode:"path"`
}
func Downloaded(torrent *Torrent, peer peer.Peer, idx int, infoHash []byte) (bool, error) {

	res, err := peer.SendHandshake(infoHash)
	if err != nil {
		return false, err
	}
	//wait for bitfield
	res, err = peer.WaitForMessage(BITFIELD)
	if err != nil {
		return false, err
	}
	bitfield := []byte{}
	for _, x := range res {
		binRep := fmt.Sprintf("%08b", x)
		bitfield = append(bitfield, []byte(binRep)...)
	}
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

	length := torrent.Info.PieceLength
    numPieces := torrent.Info.Length/torrent.Info.PieceLength

	if idx == numPieces-1 {
		length = torrent.Info.Length % torrent.Info.PieceLength
	}
	peer.DownloadPiece([]byte(torrent.Info.Pieces), length, idx)

	return true, nil
}

func spawnCr(torrent *Torrent, wg *sync.WaitGroup, peerCh chan peer.Peer, i int, infoHash []byte) {
	defer wg.Done()

	peer := <-peerCh
	err := peer.Connect()
	if err != nil {
		return
	}
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
	if err != nil {
		panic(err)
	}
    if len(torrent.Info.Files) == 0{
        torrent.Info.Files = append(torrent.Info.Files,File{
            Length: torrent.Info.Length,
            Path: []string{torrent.Info.Name},
        })

    }else{
        os.Mkdir(torrent.Info.Name , os.ModePerm)
    }
    for _,fs := range torrent.Info.Files{
        numPieces := fs.Length/torrent.Info.PieceLength
	    for i := 0; i < numPieces ; i++ {
	    	wg.Add(1)
	    	go spawnCr(torrent, &wg, peerCh, i, infoHash)
	    }
	    wg.Wait()
	    pieces, err := os.ReadDir("pieces")
	    if err != nil {
	    	panic(err)
	    }



	    finalPiece, _ := os.OpenFile(fs.Path[0], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	    for _, x := range pieces {
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

    var decoded = TrackerResponse{}
	err = bencode.Unmarshal(resp.Body,&decoded)
	if err != nil {
		return nil, err
	}

    
	return peer.GetAllPeers(decoded.Peers), nil
}

func (tr *Torrent) toMap() map[string]interface{}{
    mp := make(map[string]interface{})
    mp["name"] = tr.Info.Name
    mp["length"] = tr.Info.Length
    mp["piece length"] = tr.Info.PieceLength
    mp["pieces"] = tr.Info.Pieces

    if len(tr.Info.Files) != 0{
        files := make([]map[string]interface{},0)
        for i:=0 ; i < len(tr.Info.Files) ; i++{
            temp := make(map[string]interface{})
            temp["length"] = tr.Info.Files[i].Length
            temp["path"] = tr.Info.Files[i].Path
            files = append(files, temp)
        }
        mp["files"] = files
    }

    return mp
}

func (tr *Torrent) InfoHash() ([]byte, error) {

	h := sha1.New()
    

	err := bencode.Marshal(h,tr.toMap())
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func NewTorrent(torrentFile string) (*Torrent, error) {

    var torrent = Torrent{}
	file, err := os.Open(torrentFile)
	if err != nil {
		return nil, err
	}
    defer file.Close()
    
    bencode.Unmarshal(file,&torrent)

	return &torrent, nil
}
