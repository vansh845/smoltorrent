package peer

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
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

func GetAllPeers(peers string) []Peer{
    var res = []Peer{}
    for i:= 0 ; i < len(peers) ; i += 6{
        res = append(res, New([]byte(peers[i:i+6])))
    }  
    return res
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

func New(peer []byte) Peer {
	port := int(binary.BigEndian.Uint16([]byte(peer[4:])))
	return Peer{
		IpAddr: peer[:4],
		Port:   strconv.Itoa(port),
	}
}

func (p *Peer) toString() string {
	return p.IpAddr.String() + ":" + p.Port
}

func GeneratePeerId() []byte {
	peerId := make([]byte, 20)
	rand.Read(peerId)
	return peerId
}

type Peers chan Peer

type Peer struct {
	IpAddr net.IP
	Port   string
	Conn   net.Conn
}

func (peer *Peer) HasPiece(idx int, binRep string) bool {

	if binRep[idx] == '1' {

		return true
	}
	return false
}

func (peer *Peer) DownloadPiece(hashes []byte, length, index int) []byte {

	blockSize := 16 * 1024
	nPieces := int(float64(len(hashes)) / 20)
	blocks := int(math.Ceil(float64(length) / float64(blockSize)))

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

	currPiece := hashes[index*20 : (index+1)*20]

	if !bytes.Equal(h.Sum(nil), currPiece) {
		fmt.Printf("piece %d failed, hashes didn't match...\n",index+1)
	} else {
		fmt.Printf("piece %d downloaded out of %d...\n", index+1, nPieces)
	}
	fd, err := os.Create(fmt.Sprintf("%s%d", "pieces/piece", index))
	fd.Write(piece)
	return piece

}
func (p *Peer) Connect() error {
	conn, err := net.DialTimeout("tcp", p.toString(), 5*time.Second)
	if err != nil {
		return err
	}
	p.Conn = conn
	return nil
}

func (p *Peer) ToString() string {
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
	_, err := p.Conn.Write(buff)
	return err

}
func (peer *Peer) SendHandshake(infoHash []byte) ([]byte, error) {

	peerId := GeneratePeerId()

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
		return nil, err
	}

	buff := make([]byte, n)
	n, err = peer.Conn.Read(buff)
	if err != nil {
		return nil, err
	}

	return buff[:n], nil
}
