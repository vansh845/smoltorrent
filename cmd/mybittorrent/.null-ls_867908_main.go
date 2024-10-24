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

	pl := inDict["piece length"].(string)
	pieceLength, err := strconv.Atoi(pl)
	if err != nil {
		return nil, err
	}
	pieceHash := inDict["pieces"].(string)
	count := len(pieceHash) / 20

	var piece Piece = Piece{
		Length: pieceLength,
		Hashes: pieceHash,
		count:  count,
	}

	l, err := strconv.Atoi(inDict["length"].(string))
	if err != nil {
		return nil, err
	}
	var info Info = Info{
		Length: l,
		Name:   inDict["name"].(string),
		Piece:  piece,
	}

	torrent := &Torrent{
		Announce:  dict["announce"].(string),
		CreatedBy: dict["created by"].(string),
		Info:      info,
	}

	return torrent, nil
}

type Peer struct {
	IpAddr net.IP
	Port   string
	Conn   net.Conn
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
	return p.IpAddr + ":" + p.Port
}

func (p *Peer) sendMessage(message PeerMessage, payload []byte) error {

	msgLength := 1
	if payload != nil {
		msgLength += len(payload)
	}
	var buff []byte = make([]byte, 5)
	binary.BigEndian.PutUint32(buff[:4], uint32(msgLength))
	buff[4] = byte(message)

	_, err := p.Conn.Write(buff)

	if err != nil {
		return err
	}
	return nil

}

type Torrent struct {
	Announce  string
	CreatedBy string
	Info      Info
}

type Info struct {
	Length int
	Name   string
	Piece  Piece
}

type Piece struct {
	Length int
	Hashes string
	count  int
}

func (tr *Torrent) InfoHash() ([]byte, error) {

	h := sha1.New()
	err := bencode.Marshal(h, tr.Info)
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
func (tr *Torrent) discoverPeers() ([]Peer, error) {

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
	fmt.Println(respMap)
	ps := respMap["peers"].(string)
	prs := []byte(ps)
	peers := make([]Peer, 0)
	for i := 0; i < len(peers); i += 6 {

		peers = append(peers, NewPeer(prs[i*6:(i+1)*6]))

	}
	return peers, nil
}

func (peer *Peer) sendHandshake(tr *Torrent) (net.Conn, []byte) {

	peerId := generatePeerId()
	infoHash, _ := tr.InfoHash()

	conn, err := net.Dial("tcp", peer.toString())

	//send BitTorrent protocol to peer
	msg := make([]byte, 0)
	msg = append(msg, 19)
	protocol := []byte("BitTorrent protocol")
	msg = append(msg, protocol...)
	reserved := make([]byte, 8)
	msg = append(msg, reserved...)

	msg = append(msg, infoHash...)
	msg = append(msg, peerId...)
	if err != nil {
		panic(err)
	}
	n, err := conn.Write(msg)
	if err != nil {
		panic(err)
	}

	buff := make([]byte, n)
	_, err = conn.Read(buff)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return conn, buff
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

		fd, err := os.Open(torrentFile)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer fd.Close()
		decoder := NewDecoder(fd)
		b, _ := decoder.ReadByte()
		if b != 'd' {
			fmt.Println("not a dictionary")
			os.Exit(1)
		}

		mp, err := decoder.readDict()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		info, ok := mp["info"].(map[string]interface{})

		if !ok {
			fmt.Println("something went wrong!")
			os.Exit(1)
		}

		s := toStruct(mp)
		fmt.Println(info)
		// pieces := info["pieces"].([]byte)

		fmt.Printf("Tracker URL: %s\n", s.Url)
		fmt.Printf("Length: %d\n", s.TorrentInfo.InfoLength)
		h := sha1.New()
		err = bencode.Marshal(h, info)
		if err != nil {
			fmt.Print(err)
			os.Exit(1)
		}
		fmt.Printf("Info Hash: %x\n", h.Sum(nil))
		fmt.Printf("Piece Length: %d\n", s.TorrentInfo.PieceLength)
		fmt.Printf("Piece Hashes: %x\n", s.TorrentInfo.Pieces)

	} else if command == "peers" {

		torrentFile := os.Args[2]
		torrent, err := NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}
		peers, err := torrent.discoverPeers()
		if err != nil {
			panic(err)
		}

		for _, x := range peers {
			fmt.Println(x.toString())
		}

	} else if command == "handshake" {

		ipaddr := os.Args[3]
		torrentFile := os.Args[4]
		torrent, err := NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}

		peer := NewPeer([]byte(ipaddr))

		_, buff := peer.sendHandshake(torrent)

		fmt.Printf("Peer ID: %s\n", hex.EncodeToString(buff[48:]))

	} else if command == "download_piece" {

		torrentFile := os.Args[4]
		index, _ := strconv.Atoi(os.Args[5])
		torrent, err := NewTorrent(torrentFile)
		if err != nil {
			panic(err)
		}
		peers, err := torrent.discoverPeers()
		if err != nil {
			panic(err)
		}
		peer := peers[0]

		conn, buff := peer.sendHandshake(torrent)
		fmt.Println(string(buff))

		tmp := make([]byte, 16)
		// read bitfield message
		n, err := conn.Read(tmp)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("read %d bytes\n", n)
		fmt.Println(tmp[:n])
		// send intereseted
		interested := make([]byte, 4)
		binary.BigEndian.PutUint32(interested, 1)
		interested = append(interested, 2)
		conn.Write(interested)

		// wait for unchoke
		n, err = conn.Read(tmp)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("%d bytes read\n", n)
		fmt.Println(tmp[:n])

		// send request message

		length := info.TorrentInfo.PieceLength
		if index == info.TorrentInfo.Len-1 {
			length = info.TorrentInfo.InfoLength % info.TorrentInfo.PieceLength
		}
		blockSize := 16 * 1024

		blocks := int(math.Ceil(float64(length) / float64(blockSize)))

		ans := make([]byte, 0)
		for i := 0; i < blocks; i++ {

			request := make([]byte, 4)
			binary.BigEndian.PutUint32(request, 13)
			request = append(request, 6)
			buff := make([]byte, 4)
			binary.BigEndian.PutUint32(buff, uint32(index))
			request = append(request, buff...)
			binary.BigEndian.PutUint32(buff, uint32(i*blockSize))
			request = append(request, buff...)
			if i == blocks-1 {
				left := length - (i * blockSize)
				binary.BigEndian.PutUint32(buff, uint32(left))

			} else {
				binary.BigEndian.PutUint32(buff, uint32(blockSize))
			}

			request = append(request, buff...)

			_, err := conn.Write(request)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			resBuf := make([]byte, 4)
			_, err = conn.Read(resBuf)
			if err != nil {
				fmt.Println("error reading prefix length", err)
				os.Exit(1)
			}
			respLength := binary.BigEndian.Uint32(resBuf)
			resMesType := make([]byte, 1)
			n, err = conn.Read(resMesType)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if resMesType[0] != 7 {
				fmt.Printf("invalid message with code %d\n", resMesType[0])
				panic("bruhhh")
			}
			newBuff := make([]byte, respLength-1)

			n, err = io.ReadFull(conn, newBuff)

			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("%d bytes read\n", n)
			ans = append(ans, newBuff[8:]...)

		}
		h = sha1.New()
		n, err = h.Write(ans)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println(n, "bytes read")

		pieces := info.TorrentInfo.Pieces.(string)
		pieceHash := []byte(pieces)
		currPiece := pieceHash[index*20 : (index+1)*20]
		fmt.Println(hex.EncodeToString(h.Sum(nil)))
		fmt.Println(hex.EncodeToString(currPiece))

		if !bytes.Equal(h.Sum(nil), currPiece) {
			fmt.Println("Hashes don't match")
		} else {
			fmt.Println("Hurray!!!")
		}
		filePath := os.Args[3]
		fd, err = os.Create(filePath)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		n, err = fd.Write(ans)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("wrote %d bytes\n", n)

	} else if command == "download" {

		torrentFile := os.Args[4]

		fd, err := os.Open(torrentFile)
		if err != nil {
			panic(err)
		}
		defer fd.Close()
		decoder := NewDecoder(fd)
		b, _ := decoder.ReadByte()
		if b != 'd' {
			fmt.Println("wrong bencoded .torrent file")
			return
		}
		mp, err := decoder.readDict()
		if err != nil {
			panic(err)
		}

		inMp := mp["info"].(map[string]interface{})
		info := toStruct(mp)

		err = os.Mkdir("pieces", os.ModePerm)
		if err != nil {
			if !os.IsExist(err) {

				panic(err)
			}
		}

		wg := sync.WaitGroup{}
		peers := make([]string, 0)
		peer_id := make([]byte, 20)
		rand.Read(peer_id)
		h := sha1.New()
		err = bencode.Marshal(h, inMp)
		if err != nil {
			fmt.Print(err)
			os.Exit(1)
		}
		info_hash := h.Sum(nil)
		peerBytes := discoverPeers(mp, peer_id, info_hash)

		for i := 0; i < len(peerBytes); i = i + 6 {

			ipaddr := NewPeer(peerBytes[i : i+6])
			if ipaddr == "0.0.0.0" {
				continue
			}
			peers = append(peers, ipaddr)
		}
		fmt.Println(peers)
		for i := 0; i < info.TorrentInfo.Len; i++ {
			wg.Add(1)
			go func(wg *sync.WaitGroup) {
				defer wg.Done()

				peer := peers[i%len(peers)]
				conn, buff := sendHandshake(peer, mp)
				fmt.Println(string(buff))
				// fmt.Println(string(buff))
				tmp := make([]byte, 16)
				// read bitfield message
				n, err := conn.Read(tmp)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				fmt.Printf("read %d bytes\n", n)
				fmt.Println(tmp[:n])
				// send intereseted
				interested := make([]byte, 4)
				binary.BigEndian.PutUint32(interested, 1)
				interested = append(interested, 2)
				conn.Write(interested)

				// wait for unchoke
				n, err = conn.Read(tmp)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				fmt.Printf("%d bytes read\n", n)
				fmt.Println(tmp[:n])
				downloadPiece(conn, i, *info)
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

func NewPeer(peer []byte) Peer {

	port := int(binary.BigEndian.Uint16([]byte(peer[4:])))
	return Peer{
		IpAddr: peer[:4],
		Port:   strconv.Itoa(port),
	}
}

func downloadPiece(conn net.Conn, index int, info Torrent) []byte {

	// send request message

	length := info.Info.Length
	if index == info.TorrentInfo.Len-1 {
		length = info.TorrentInfo.InfoLength % info.TorrentInfo.PieceLength
	}
	blockSize := 16 * 1024

	blocks := int(math.Ceil(float64(length) / float64(blockSize)))

	ans := make([]byte, 0)
	for i := 0; i < blocks; i++ {

		request := make([]byte, 4)
		binary.BigEndian.PutUint32(request, 13)
		request = append(request, 6)
		buff := make([]byte, 4)
		binary.BigEndian.PutUint32(buff, uint32(index))
		request = append(request, buff...)
		binary.BigEndian.PutUint32(buff, uint32(i*blockSize))
		request = append(request, buff...)
		if i == blocks-1 {
			left := length - (i * blockSize)
			binary.BigEndian.PutUint32(buff, uint32(left))

		} else {
			binary.BigEndian.PutUint32(buff, uint32(blockSize))
		}

		request = append(request, buff...)

		_, err := conn.Write(request)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		resBuf := make([]byte, 4)
		_, err = conn.Read(resBuf)
		if err != nil {
			fmt.Println("error reading prefix length", err)
			os.Exit(1)
		}
		respLength := binary.BigEndian.Uint32(resBuf)
		resMesType := make([]byte, 1)
		n, err := conn.Read(resMesType)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if resMesType[0] != 7 {
			fmt.Printf("invalid message with code %d\n", resMesType[0])
			panic("bruhhh")
		}
		newBuff := make([]byte, respLength-1)

		n, err = io.ReadFull(conn, newBuff)

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("%d bytes read\n", n)
		ans = append(ans, newBuff[8:]...)

	}
	h := sha1.New()
	n, err := h.Write(ans)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(n, "bytes read")

	pieces := info.TorrentInfo.Pieces.(string)
	pieceHash := []byte(pieces)
	currPiece := pieceHash[index*20 : (index+1)*20]
	fmt.Println(hex.EncodeToString(h.Sum(nil)))
	fmt.Println(hex.EncodeToString(currPiece))

	if !bytes.Equal(h.Sum(nil), currPiece) {
		fmt.Println("Hashes don't match")
	} else {
		fmt.Println("Piece hash matched")
	}
	fd, err := os.Create(fmt.Sprintf("%s%d", "pieces/piece", index))
	fd.Write(ans)
	return ans

}
