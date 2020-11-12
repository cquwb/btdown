package client

import (
	"encoding/binary"
)

type messageID uint8

const (
	MsgChoke         messageID = 0
	MsgUnchoke       messageID = 1
	MsgInterested    messageID = 2
	MsgNotInterested messageID = 3
	MsgHave          messageID = 4
	MsgBitfield      messageID = 5
	MsgRequest       messageID = 6
	MsgPiece         messageID = 7
	MsgCancel        messageID = 8
)

type PeerMessage struct {
	Len  uint32    //消息长度
	Id   messageID //消息id
	Data []byte    //消息体
}

func (pm *PeerMessage) Serialize() []byte {
	l := 1 + len(pm.Data)
	buf := make([]byte, l+4)
	binary.BigEndian.PutUint32(buf[0:4], uint32(l))
	copy(buf[4:5], []byte{byte(pm.Id)})
	copy(buf[5:], pm.Data[:])
	return buf
}

func UnSerializePeerMessage(data []byte) (*PeerMessage, error) {
	msg := &PeerMessage{}
	msg.Id = messageID(data[0])
	msg.Data = data[1:]
	return msg, nil
}

type Bitfield []byte

type RequestPayload struct {
	Index  int
	Begin  int
	Length int
}

func (r *RequestPayload) Serialize() []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[0:4], uint32(r.Index))
	binary.BigEndian.PutUint32(buf[4:8], uint32(r.Begin))
	binary.BigEndian.PutUint32(buf[8:12], uint32(r.Length))
	return buf
}

type PiecePayload struct {
	Index int
	Begin int
	Block []byte
}

func UnSerializePiecePayload(data []byte) *PiecePayload {
	payload := &PiecePayload{}
	payload.Index = int(binary.BigEndian.Uint32(data[0:4]))
	payload.Begin = int(binary.BigEndian.Uint32(data[4:8]))
	payload.Block = data[8:]
	return payload
}

type TotalPiecePayload struct {
	Index int
	Block []byte
}
