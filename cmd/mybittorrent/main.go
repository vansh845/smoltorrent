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

	bencode "github.com/jackpal/bencode-go" // Available if you need it!
)

func NewDecoder(rdr io.Reader) *decoder {
	return &decoder{
		*bufio.NewReader(rdr),
	}
}

type decoder struct {
	bufio.Reader
}

func toStruct(mp map[string]interface{}) *Torrent {
	res := Torrent{}
	res.Url = mp["announce"].(string)
	res.CreatedBy = mp["created by"].(string)

	infoMp := mp["info"].(map[string]interface{})
	res.TorrentInfo = Info{
		InfoLength:  infoMp["length"].(int),
		Name:        infoMp["name"].(string),
		PieceLength: infoMp["piece length"].(int),
		Pieces:      infoMp["pieces"],
		Len:         int(math.Ceil(float64(len(infoMp["pieces"].(string))) / float64(20))),
	}
	return &res
}

type Torrent struct {
	Url         string
	CreatedBy   string
	TorrentInfo Info
}

type Info struct {
	InfoLength  int
	Name        string
	PieceLength int
	Pieces      interface{}
	Len         int
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

func sendHandshake(peer string, msg []byte) (net.Conn, []byte) {
	conn, err := net.Dial("tcp", peer)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	n, err := conn.Write(msg)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	buff := make([]byte, n)
	_, err = conn.Read(buff)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return conn, buff
}

func getHandshake(torrentFile string) ([]byte, []byte, []byte) {

	fd, err := os.Open(torrentFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer fd.Close()
	decoder := NewDecoder(fd)
	b, _ := decoder.ReadByte()
	if b == 'd' {

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

		h := sha1.New()
		err = bencode.Marshal(h, info)
		if err != nil {
			fmt.Print(err)
			os.Exit(1)
		}
		peer_id := make([]byte, 20)
		baseUrl := mp["announce"].(string)
		info_hash := h.Sum(nil)
		port := "6881"
		left := fmt.Sprintf("%d", info["length"])

		rand.Read(peer_id)
		params := url.Values{}
		params.Add("info_hash", string(info_hash))
		params.Add("peer_id", string(peer_id))
		params.Add("port", port)
		params.Add("uploaded", "0")
		params.Add("downloaded", "0")
		params.Add("left", left)
		params.Add("compact", "1")

		url := baseUrl + "?" + params.Encode()
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		decoded, err := decodeBencode(resp.Body)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		respMap := decoded.(map[string]interface{})
		peers := respMap["peers"].(string)
		peerBytes := []byte(peers)
		return peerBytes, info_hash, peer_id
	}

	return nil, nil, nil
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
		if b == 'd' {

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

			// pieces := info["pieces"].([]byte)

			fmt.Printf("Tracker URL: %s\n", mp["announce"])
			fmt.Printf("Length: %d\n", info["length"])
			h := sha1.New()
			err = bencode.Marshal(h, info)
			if err != nil {
				fmt.Print(err)
				os.Exit(1)
			}
			fmt.Printf("Info Hash: %x\n", h.Sum(nil))
			fmt.Printf("Piece Length: %d\n", info["piece length"])
			fmt.Printf("Piece Hashes: %x\n", info["pieces"])

		}
	} else if command == "peers" {

		peerBytes, _, _ := getHandshake(os.Args[2])
		fmt.Println(printPeer(peerBytes[:6]))
		fmt.Println(printPeer(peerBytes[6:12]))
		fmt.Println(printPeer(peerBytes[12:18]))

	} else if command == "handshake" {

		ipaddr := os.Args[3]

		_, info_hash, peer_id := getHandshake(os.Args[2])
		msg := make([]byte, 0)
		msg = append(msg, 19)
		protocol := []byte("BitTorrent protocol")
		msg = append(msg, protocol...)
		reserved := make([]byte, 8)
		msg = append(msg, reserved...)

		msg = append(msg, info_hash...)
		msg = append(msg, peer_id...)
		_, buff := sendHandshake(ipaddr, msg)

		fmt.Printf("Peer ID: %s\n", hex.EncodeToString(buff[48:]))

	} else if command == "download_piece" {

		torrentFile := os.Args[4]
		index, _ := strconv.Atoi(os.Args[5])
		peerBytes, _, _ := getHandshake(torrentFile)
		ipaddr := printPeer(peerBytes[12:18])
		fmt.Println(ipaddr)

		fd, err := os.Open(torrentFile)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer fd.Close()
		decoder := NewDecoder(fd)
		b, _ := decoder.ReadByte()
		if b == 'd' {

			mp, err := decoder.readDict()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			// infoMp, ok := mp["info"].(map[string]interface{})

			// if !ok {
			// 	fmt.Println("something went wrong!")
			// 	os.Exit(1)
			// }

			info := toStruct(mp)

			_, info_hash, peer_id := getHandshake(torrentFile)
			msg := make([]byte, 0)
			msg = append(msg, 19)
			protocol := []byte("BitTorrent protocol")
			msg = append(msg, protocol...)
			reserved := make([]byte, 8)
			msg = append(msg, reserved...)

			msg = append(msg, info_hash...)
			msg = append(msg, peer_id...)
			conn, buff := sendHandshake(ipaddr, msg)
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
			h := sha1.New()
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
			fd, err := os.Create(filePath)
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

		}
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}

func printPeer(peer []byte) string {
	sb := ""
	for i, x := range peer[:4] {

		sb += strconv.Itoa(int(x))
		if i != 3 {
			sb += "."
		}
	}
	sb += ":"
	port := binary.BigEndian.Uint16([]byte(peer[4:]))
	sb += strconv.Itoa(int(port))
	return sb
}
