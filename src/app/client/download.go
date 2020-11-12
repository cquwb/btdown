package client

import (
	l4g "github.com/ivanabc/log4go"
	"io"
	"net"
)

const (
	MaxBackLog int = 5 //最大的下载队列 5个
	BlockSize  int = 16384
)

type Peer struct {
	ip   net.IP
	port uint16
}

type PeerDownload struct {
	beginTime  int64
	index      int    //piece index
	size       int    //piece size
	backlog    int    //已经开始的请求个数
	requested  int    //已经开始请求的数据大小
	downloaded int    //已经下载的数据大小
	data       []byte //已经下载的数据。是一个完整的byte
}

type PieceData struct {
	index int    //piece 的index
	data  []byte //实际的数据
}

type HandShakeMessage struct {
	Pstr     string
	InfoHash [20]byte
	PeerId   [20]byte
}

func (h *HandShakeMessage) Serialize() []byte {
	pstrlen := len(h.Pstr)
	bufLen := 49 + pstrlen
	buf := make([]byte, bufLen)
	buf[0] = byte(pstrlen)
	copy(buf[1:], h.Pstr)
	copy(buf[1+pstrlen+8:], h.InfoHash[:])
	copy(buf[1+pstrlen+8+20:], h.PeerId[:])
	return buf
}

//怎么从切片里赋值给数组？
func UnSerializeHandShake(r io.Reader, msg *HandShakeMessage) error {
	data := make([]byte, 1)
	_, err := r.Read(data)
	if err != nil {
		return err
	}
	pstrlen := int(int8(data[0]))
	data2 := make([]byte, int(pstrlen))
	readlen := 0
	for readlen < pstrlen {
		n, err := r.Read(data2[readlen:])
		if err != nil {
			return err
		}
		readlen += n
	}
	l4g.Debug("read pstr :%s", data2)
	resverd := [8]byte{}
	readlen = 0
	for readlen < 8 {
		n, err := r.Read(resverd[readlen:])
		if err != nil {
			return err
		}
		readlen += n
	}
	l4g.Debug("read resverd :%+v", resverd)

	hash := [20]byte{}
	readlen = 0
	for readlen < 20 {
		n, err := r.Read(hash[readlen:])
		if err != nil {
			return err
		}
		readlen += n
	}
	l4g.Debug("read infohash :%+v", hash)
	peerId := [20]byte{}
	readlen = 0
	for readlen < 20 {
		n, err := r.Read(peerId[readlen:])
		if err != nil {
			return err
		}
		readlen += n
	}
	msg.InfoHash = hash
	msg.PeerId = peerId
	return nil
}
