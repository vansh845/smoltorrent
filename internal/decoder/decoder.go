package decoder

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

func DecodeBencode(bencodedRdr io.Reader) (interface{}, error) {
	d := NewDecoder(bencodedRdr)
	text, err := d.Decode()
	return text, err
}
type decoder struct {
	bufio.Reader
}

func NewDecoder(rdr io.Reader) *decoder {
	return &decoder{
		*bufio.NewReader(rdr),
	}
}

func (d *decoder) Decode() (text interface{}, err error) {
	text, err = d.readType()
	return text, err
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
