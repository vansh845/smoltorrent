package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	bencode "github.com/jackpal/bencode-go" // Available if you need it!
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

func NewDecoder(rdr io.Reader) *decoder {
	return &decoder{
		*bufio.NewReader(rdr),
	}
}

type decoder struct {
	bufio.Reader
}

// reads .torrent file and returns Torrent struct
func NewTorrent(torrentFile string) (*Torrent, error) {

	file, err := os.Open(torrentFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := NewDecoder(file)
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
func NewPeerFromString(peer string) Peer {
	tmp := strings.Split(peer, ":")
	ip := tmp[0]
	port := tmp[1]
	tmpByte := strings.Split(ip, ".")
	ipBytes := make([]byte, 0)
	for i := 0; i < len(tmpByte); i++ {
		by, _ := strconv.Atoi(tmpByte[i])
		ipBytes = append(ipBytes, byte(by))
	}
	return Peer{
		IpAddr: ipBytes,
		Port:   port,
	}
}

func NewPeer(peer []byte) Peer {
	port := int(binary.BigEndian.Uint16([]byte(peer[4:])))
	return Peer{
		IpAddr: peer[:4],
		Port:   strconv.Itoa(port),
	}
}

type Peer struct {
	IpAddr net.IP
	Port   string
	Conn   net.Conn
}

func (peer *Peer) DownloadPiece(torrent *Torrent, index int) []byte {

	length := torrent.Info.Piece.Length
	if index == torrent.Info.Piece.Count-1 {
		length = torrent.Info.Length % torrent.Info.Piece.Length
	}
	blockSize := 16 * 1024

	blocks := int(math.Ceil(float64(length) / float64(blockSize)))
	fmt.Println(length)
	fmt.Println(blocks)

	piece := make([]byte, 0)
	for i := 0; i < blocks; i++ {

		payload := make([]byte, 12)
		binary.BigEndian.PutUint32(payload[:4], uint32(index))
		binary.BigEndian.PutUint32(payload[4:8], uint32(i*blockSize))
		if i == blocks-1 {
			left := length - (i * blockSize)
			binary.BigEndian.PutUint32(payload[8:12], uint32(left))
		} else {
			binary.BigEndian.PutUint32(payload[8:12], uint32(blockSize))
		}

		err := peer.SendMessage(REQUEST, payload)
		if err != nil {
			panic(err)
		}

		res, err := peer.WaitForMessage(PIECE)

		if err != nil {
			panic(err)
		}
		piece = append(piece, res[8:]...)

	}
	h := sha1.New()
	_, err := h.Write(piece)
	if err != nil {
		panic(err)
	}

	pieces := torrent.Info.Piece.Hashes
	pieceHash := []byte(pieces)
	currPiece := pieceHash[index*20 : (index+1)*20]
	fmt.Println(hex.EncodeToString(h.Sum(nil)))
	fmt.Println(hex.EncodeToString(currPiece))

	if !bytes.Equal(h.Sum(nil), currPiece) {
		fmt.Println("Hashes don't match")
	} else {
		fmt.Println("Hashes matched!")
	}
	fd, err := os.Create(fmt.Sprintf("%s%d", "pieces/piece", index))
	fd.Write(piece)
	return piece

}
func (p *Peer) Connect() error {
	conn, err := net.Dial("tcp", p.toString())
	if err != nil {
		return err
	}
	p.Conn = conn
	return nil
}

func (p *Peer) toString() string {
	return p.IpAddr.String() + ":" + p.Port
}

// blocks until recieves message from
func (p *Peer) WaitForMessage(message PeerMessage) ([]byte, error) {
	buff := make([]byte, 4)

	_, err := p.Conn.Read(buff)
	if err != nil {
		return nil, err
	}
	msgSize := binary.BigEndian.Uint32(buff)
	buff = make([]byte, 1)
	_, err = p.Conn.Read(buff)
	if err != nil {
		return nil, err
	}

	if int(buff[0]) != int(message) {
		return nil, fmt.Errorf("message sent by peer %d , expected %d\n", int(buff[0]), int(message))
	}
	buff = make([]byte, msgSize-1)

	_, err = io.ReadFull(p.Conn, buff)
	if err != nil {
		return nil, err
	}

	return buff, nil
}

func (p *Peer) SendMessage(message PeerMessage, payload []byte) error {

	msgLength := 1

	var buff []byte = make([]byte, 5)
	buff[4] = byte(message)

	if payload != nil {
		msgLength += len(payload)
		buff = append(buff, payload...)
	}

	binary.BigEndian.PutUint32(buff[:4], uint32(msgLength))
	fmt.Println(buff)
	_, err := p.Conn.Write(buff)
	return err

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

func (tr *Torrent) InfoHash() ([]byte, error) {

	h := sha1.New()
	err := bencode.Marshal(h, tr.mp)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func generatePeerId() []byte {
	peerId := make([]byte, 20)
	rand.Read(peerId)
	return peerId
}

func (d *decoder) readDict() (map[string]interface{}, error) {
	var res = make(map[string]interface{})
	for {
		b, err := d.ReadByte()
		if err != nil {
			return res, err
		}
		if b == 'e' {
			break
		}
		err = d.UnreadByte()
		if err != nil {
			return res, err
		}
		key, err := d.readString()
		if err != nil {
			return res, err
		}
		value, err := d.readType()
		if err != nil {
			return res, err
		}
		res[key] = value
	}
	return res, nil

}

func (d *decoder) readIntUntil(delem byte) (int, error) {
	slc, err := d.ReadSlice(delem)
	str := string(slc[:len(slc)-1])
	if err != nil {
		return 0, err
	}
	number, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(number), err
}

func (d *decoder) readString() (text string, err error) {
	length, err := d.readIntUntil(':')
	if err != nil {
		return "", err
	}
	sb := strings.Builder{}
	for i := 0; i < length; i++ {
		c, err := d.ReadByte()
		if err != nil {
			return "", nil
		}
		sb.WriteByte(c)
	}
	text = sb.String()
	return text, err
}

func (d *decoder) readInt() (int, error) {
	ans, err := d.readIntUntil('e')
	if err != nil {
		return 0, err
	}
	return ans, nil
}

func (d *decoder) readList() ([]interface{}, error) {
	var res []interface{} = make([]interface{}, 0)
	for {
		ch, err := d.ReadByte()
		if err != nil {
			return nil, err
		}
		if ch == 'e' {
			break
		}
		err = d.UnreadByte()
		if err != nil {
			return nil, err
		}
		text, err := d.readType()
		if err != nil {
			return nil, err
		}
		res = append(res, text)
	}
	return res, nil

}

func (d *decoder) readType() (text interface{}, err error) {

	fb, err := d.ReadByte()
	if err != nil {
		return "", err
	}
	switch fb {
	case 'i':
		text, err = d.readInt()
	case 'l':
		text, err = d.readList()
	case 'd':
		text, err = d.readDict()
	default:
		err = d.UnreadByte()
		if err != nil {
			return "", err
		}
		text, err = d.readString()
	}
	return text, err
}

func (d *decoder) Decode() (text interface{}, err error) {
	text, err = d.readType()
	return text, err
}

// Ensures gofmt doesn't remove the "os" encoding/json import (feel free to remove this!)
var _ = json.Marshal

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345
func decodeBencode(bencodedRdr io.Reader) (interface{}, error) {
	d := NewDecoder(bencodedRdr)
	text, err := d.Decode()
	return text, err
}

// sends request to announce and returns list of peers
func (tr *Torrent) DiscoverPeers() ([]Peer, error) {

	infoHash, err := tr.InfoHash()
	peerId := generatePeerId()
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

	decoded, err := decodeBencode(resp.Body)
	if err != nil {
		return nil, err
	}
	respMap := decoded.(map[string]interface{})
	ps := respMap["peers"].(string)
	peerByte := []byte(ps)
	peers := make([]Peer, 0)
	for i := 0; i < len(peerByte); i += 6 {

		peer := NewPeer(peerByte[i : i+6])
		if peer.IpAddr.String() == "0.0.0.0" {
			continue
		}
		peers = append(peers, peer)

	}
	return peers, nil
}

// initiates a handshake with peer
func (peer *Peer) sendHandshake(tr *Torrent) []byte {

	peerId := generatePeerId()
	infoHash, _ := tr.InfoHash()

	//send BitTorrent protocol to peer
	msg := make([]byte, 0)
	msg = append(msg, 19)
	protocol := []byte("BitTorrent protocol")
	msg = append(msg, protocol...)
	reserved := make([]byte, 8)
	msg = append(msg, reserved...)

	msg = append(msg, infoHash...)
	msg = append(msg, peerId...)
	n, err := peer.Conn.Write(msg)
	if err != nil {
		panic(err)
	}

	buff := make([]byte, n)
	n, err = peer.Conn.Read(buff)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return buff[:n]
}

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	// fmt.Println("Logs from your program will appear here!")

	command := os.Args[1]

	if command == "decode" {
		// Uncomment this block to pass the first stage
		//
		bencodedValue := os.Args[2]

		decoded, err := decodeBencode(strings.NewReader(bencodedValue))
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	} else if command == "info" {

		torrentFile := os.Args[2]

		torrent, err := NewTorrent(torrentFile)

		if err != nil {
			panic(err)
		}
		infoHash, err := torrent.InfoHash()
		fmt.Printf("Tracker URL: %s\n", torrent.Announce)
		fmt.Printf("Length: %d\n", torrent.Info.Length)
		fmt.Printf("Info Hash: %x\n", infoHash)
		fmt.Printf("Piece Length: %d\n", torrent.Info.Piece.Length)
		fmt.Printf("Piece Hashes: %x\n", torrent.Info.Piece.Hashes)

	} else if command == "peers" {

		torrentFile := os.Args[2]
		torrent, err := NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}
		peers, err := torrent.DiscoverPeers()
		if err != nil {
			panic(err)
		}

		for _, x := range peers {
			fmt.Println(x.toString())
		}

	} else if command == "handshake" {

		ipaddr := os.Args[3]
		torrentFile := os.Args[2]
		torrent, err := NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}

		peer := NewPeerFromString(ipaddr)

		err = peer.Connect()
		if err != nil {
			panic(err)
		}
		buff := peer.sendHandshake(torrent)

		fmt.Printf("Peer ID: %s\n", hex.EncodeToString(buff[48:]))

	} else if command == "download_piece" {

		torrentFile := os.Args[4]
		index, _ := strconv.Atoi(os.Args[5])
		torrent, err := NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}
		peers, err := torrent.DiscoverPeers()
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
		buff := peer.sendHandshake(torrent)
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

		// wait for unchoke
		res, err = peer.WaitForMessage(UNCHOKE)
		if err != nil {
			panic(err)
		}

		fmt.Println(res)
		// send request message

		piece := peer.DownloadPiece(torrent, index)

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

	} else if command == "download" {

		torrentFile := os.Args[4]

		torrent, err := NewTorrent(torrentFile)
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
				buff := peer.sendHandshake(torrent)
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
				peer.DownloadPiece(torrent, i)
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
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
