package peer

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
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

type Peer struct {
	IpAddr net.IP
	Port   string
	Conn   net.Conn
}

func (peer *Peer) DownloadPiece(hashes []byte, length, index int) []byte {

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

	currPiece := hashes[index*20 : (index+1)*20]
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

		if strings.Contains(err.Error(), "unsupported protocol scheme") {
			conn, err = net.Dial("udp", p.toString())
			if err != nil {
				return err
			}
		}
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
	fmt.Println(buff)
	_, err := p.Conn.Write(buff)
	return err

}
func (peer *Peer) SendHandshake(infoHash []byte) []byte {

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
