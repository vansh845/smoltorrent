package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

func sendHandshake(peer string, msg []byte) {
	conn, err := net.Dial("tcp", peer)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()
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
	fmt.Printf("Peer ID: %s\n", hex.EncodeToString(buff[48:]))

}

func getHandshake() ([]byte, []byte, []byte) {

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
			fmt.Printf("Piece Hashes: %x", info["pieces"])

		}
	} else if command == "peers" {

		peerBytes, _, _ := getHandshake()
		fmt.Println(printPeer(peerBytes[:6]))
		fmt.Println(printPeer(peerBytes[6:12]))
		fmt.Println(printPeer(peerBytes[12:18]))

	} else if command == "handshake" {

		ipaddr := os.Args[3]

		_, info_hash, peer_id := getHandshake()
		msg := make([]byte, 0)
		msg = append(msg, 19)
		protocol := []byte("BitTorrent protocol")
		msg = append(msg, protocol...)
		reserved := make([]byte, 8)
		msg = append(msg, reserved...)

		msg = append(msg, info_hash...)
		msg = append(msg, peer_id...)
		sendHandshake(ipaddr, msg)

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
