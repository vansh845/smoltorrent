package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	// bencode "github.com/jackpal/bencode-go" // Available if you need it!
)

func NewDecoder(bencodedString string) *decoder {
	return &decoder{
		*bufio.NewReader(strings.NewReader(bencodedString)),
	}
}

type decoder struct {
	bufio.Reader
}

func (d *decoder) readDict() (interface{}, error) {
	var res = make(map[string]interface{})
	for {
		d.readType()
	}
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
func decodeBencode(bencodedString string) (interface{}, error) {
	d := NewDecoder(bencodedString)
	text, err := d.Decode()
	return text, err
}

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	// fmt.Println("Logs from your program will appear here!")

	command := os.Args[1]

	if command == "decode" {
		// Uncomment this block to pass the first stage
		//
		bencodedValue := os.Args[2]

		decoded, err := decodeBencode(bencodedValue)
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
