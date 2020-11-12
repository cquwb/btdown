package client

import (
	"app/torrent"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	l4g "github.com/ivanabc/log4go"
)

//每一个链接
type PeerClient struct {
	id        int
	peer      *torrent.Peer
	conn      net.Conn
	handshake bool          //是否握手了。
	choke     bool          //是否被chock了
	bitfield  Bitfield      //这个peer所有的peer ？？？？
	download  *PeerDownload //已经开始下载的个数 一次可以下载多个吗？
	timer     *time.Ticker
	msgQueue  chan *PeerMessage
	ctx       context.Context
	Close     chan struct{}
	t         *torrent.Torrent
	pieceChan chan *TotalPiecePayload
	workQueue chan *Work
}

func NewPeerClient(id int, peer *torrent.Peer, t *torrent.Torrent, pieceChan chan *TotalPiecePayload) *PeerClient {
	//num := len(t.PiecesHash)
	//bitfield := make([]byte, (num/8 + 1))
	//初始化
	return &PeerClient{
		id:        id,
		peer:      peer,
		handshake: false,
		choke:     true,
		msgQueue:  make(chan *PeerMessage, 1000),
		Close:     make(chan struct{}, 0),
		//bitfield:  bitfield,
		pieceChan: pieceChan,
		t:         t,
	}

}

func (p *PeerClient) Run(ctx context.Context, wg *sync.WaitGroup, workQueue chan *Work) {
	l4g.Debug("peer:%s Run!", p.Identify())
	p.ctx = ctx
	p.workQueue = workQueue
	p.timer = time.NewTicker(100 * time.Millisecond)
	timeTotal := 0
	//不设置超时，等了2分钟左右
	conn, err := net.DialTimeout("tcp", p.peer.Addr(), 15*time.Second)
	if err != nil {
		l4g.Error("peer connect error %s", err)
		goto OUT
	}
	p.conn = conn
	p.SendHandShakeMsg()
	p.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	if err := p.ReceiveHandShakeMsg(); err != nil {
		l4g.Error("peer:%s receive handshake msg error %s", p.Identify(), err)
		goto OUT
	}
	p.conn.SetReadDeadline(time.Time{})
	l4g.Info("peer:%s handshake success", p.Identify())
	p.SendUnchoke()
	p.SendInterested()
	go p.ReadMsgLoop()
	//go WriteLoop()
	//创建一个定时器 ,
	for {
		//l4g.Info("peer:%s for debug", p.Identify())
		select {
		case msg := <-p.msgQueue:
			l4g.Debug("peer:%s get read msg %+v left:%d", p.Identify(), msg.Id, len(p.msgQueue))
			//处理消息
			p.DealMessage(msg)
		case <-p.ctx.Done():
			l4g.Info("peer:%s end", p.Identify())
			goto OUT
		case <-p.timer.C:
			//l4g.Info("peer:%s timer :%d", p.Identify(), now.Unix())
			timeTotal++
			if timeTotal%600 == 0 {
				p.SendKeepAlive()
			}
			if timeTotal%10 == 0 {
				p.CheckDownload()
			}
			if p.choke || p.download != nil {
				break
			}
			select {
			case work := <-workQueue:
				//TODO 这里没准备好不弄
				index := work.Index
				if p.choke == true {
					l4g.Debug("peer %s, get work but choke %d", p.Identify(), index)
					p.workQueue <- work //把工作丢回去
					break
					//continue
				}
				if !p.CheckHaveBitField(index) {
					l4g.Info("peer %s, get work but not bit field %d", p.Identify(), index)
					p.workQueue <- work //把工作丢回去
					break
					//continue
				}
				if !p.BeginDownload(index) {
					l4g.Info("peer %s, get work but begin download error %d", p.Identify(), index)
					p.workQueue <- work
					break
					//continue
				}
			default:
				//do nothing, 不阻塞
			}
		case <-p.Close:
			l4g.Info("peer:%s close", p.Identify())
			goto OUT
		}
	}
OUT:
	l4g.Info("peer:%s out", p.Identify())
	wg.Done()
}

func (p *PeerClient) SetChock(status bool) {}

//添加新的piece index
func (p *PeerClient) AddNewBitField(index int) {}

//获取是否拥有某个piece
func (p *PeerClient) CheckHaveBitField(index int) bool {
	if p.bitfield == nil {
		return false
	}
	i := index / 8
	j := index % 8
	if len(p.bitfield) > i {
		b := p.bitfield[i]
		return (b>>(7-j))&1 != 0
	} else {
		return false
	}
}

//开始下载某个piece
func (p *PeerClient) StartDownloadPiece(index int) {
}

func (p *PeerClient) SetPiece(index int) {
	if p.bitfield == nil {
		return
	}
	if index >= len(p.t.PiecesHash) {
		return
	}

	//准备好N个队列下载。
	i := index / 8
	j := index % 8
	p.bitfield[i] |= (1 >> j)
}

//发送消息
func (p *PeerClient) SendMsg(id messageID, data interface{}) {
}

func (p *PeerClient) SendUnchoke() {
	l4g.Debug("peer:%s send unchoke msg ", p.Identify())
	message := &PeerMessage{
		Id: MsgUnchoke,
	}
	p.conn.Write(message.Serialize())
}

func (p *PeerClient) SendInterested() {
	l4g.Debug("peer:%s send interested msg ", p.Identify())
	message := &PeerMessage{
		Id: MsgInterested,
	}
	p.conn.Write(message.Serialize())
}

func (p *PeerClient) SendKeepAlive() {
	l4g.Debug("peer:%s send keepalive msg ", p.Identify())
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data[0:4], 0)
	p.conn.Write(data)
}

//发送消息
func (p *PeerClient) SendHandShakeMsg() {
	l4g.Debug("peer:%s send handshake msg ", p.Identify())
	msg := &HandShakeMessage{
		Pstr:     "BitTorrent protocol",
		InfoHash: p.t.InfoHash,
		PeerId:   p.t.PeerId,
	}
	p.conn.Write(msg.Serialize())
}

func (p *PeerClient) ReceiveHandShakeMsg() error {
	msg := &HandShakeMessage{}
	if err := UnSerializeHandShake(p.conn, msg); err != nil {
		return err
	}
	if string(p.t.InfoHash[:]) != string(msg.InfoHash[:]) {
		return errors.New(fmt.Sprintf("handshake error not same infohash,send:%s, receive:%s", string(p.t.InfoHash[:]), string(msg.InfoHash[:])))
	}
	/*
		if string(p.t.PeerId[:]) != string(msg.PeerId[:]) {
			return errors.New(fmt.Sprintf("handshake error not same peer id, send:%s receive:%s", string(p.t.PeerId[:]), string(msg.PeerId[:])))
		}
	*/
	return nil
}

//发送request
func (p *PeerClient) SendRequestMsg(index, begin, length int) bool {
	l4g.Debug("peer:%s send request msg %d %d %d", p.Identify(), index, begin, length)
	payload := &RequestPayload{
		Index:  index,
		Begin:  begin,
		Length: length,
	}
	message := &PeerMessage{
		Id:   MsgRequest,
		Data: payload.Serialize(),
	}
	_, err := p.conn.Write(message.Serialize())
	if err != nil {
		l4g.Error("peer:%s send msg error %s", p.Identify(), err)
		return false
	}
	return true
}

//循环接收消息
func (p *PeerClient) ReadMsgLoop() {
	//keepalive 时间是2分钟
	//超时是5分钟

	for {
		data := make([]byte, 4)
		_, err := io.ReadFull(p.conn, data)
		if err != nil {
			p.Stop()
			l4g.Error("peer %s read error %s", p.peer.Addr(), err)
			return
		}
		l := binary.BigEndian.Uint32(data)
		if l == 0 {
			l4g.Info("peer:%s receive 0 length msg", p.Identify())
			continue
		}
		data2 := make([]byte, l)
		if _, err := io.ReadFull(p.conn, data2); err != nil {
			p.Stop()
			l4g.Error("peer %s read data error %s", p.peer.Addr(), err)
			return
		}

		if msg, err := UnSerializePeerMessage(data2); err == nil {
			l4g.Debug("peer:%s receive new msg :%+v", p.Identify(), msg.Id)
			p.msgQueue <- msg
			l4g.Debug("peer:%s receive new msg msgQueue len:%d", p.Identify(), len(p.msgQueue))
		} else {
			p.Stop()
			l4g.Error("peer:%s unserialize peer msg error:%v %s", p.Identify(), data2, err)
			return
		}
	}

}

func (p *PeerClient) Stop() {
	if p.download != nil {
		p.workQueue <- &Work{p.download.index}
	}
	close(p.Close)
}

//读取一个消息
func (p *PeerClient) ReadMsg() (*PeerMessage, error) {
	return nil, nil
}

func (p *PeerClient) ReceivePieceData(index int, offset int, data []byte) {

}

func (p *PeerClient) DealMessage(msg *PeerMessage) {
	l4g.Debug("peer:%s DealMessage %d ", p.Identify(), msg.Id)
	switch msg.Id {
	case MsgChoke:
		p.choke = true
		l4g.Info("peer %s has set choke", p.peer.Addr())
	case MsgUnchoke:
		p.choke = false
		l4g.Info("peer %s has set unchoke", p.peer.Addr())
	case MsgInterested:
	case MsgNotInterested:
	case MsgBitfield:
		l4g.Debug("peer:%s get bit field data :%+v", p.Identify(), msg.Data)
		p.bitfield = msg.Data
		//copy(p.bitfield, msg.Data)
	case MsgPiece:
		payload := UnSerializePiecePayload(msg.Data)
		l4g.Info("peer:%s get piece from peer:%d, index:%d begin:%d len:%d", p.Identify(), p.id, payload.Index, payload.Begin, len(payload.Block))
		if p.download == nil {
			l4g.Error("peer:%s get piece ereror, no download", p.Identify())
			return
		}
		if p.download.index != payload.Index {
			l4g.Error("peer:%s get piece ereror, index wrong:download index:%d, receive index:%d", p.Identify(), p.download.index, payload.Index)
			return
		}
		p.NewDownload(payload)
		//p.pieceChan <- payload
	}
}

func (p *PeerClient) Identify() string {
	return p.peer.Addr()
}

func (p *PeerClient) BeginDownload(index int) bool {
	if p.download != nil {
		l4g.Error("peer:%s begin download index:%d but in download:%d", p.Identify(), index, p.download.index)
		return false //已经开始下载了
	}
	l4g.Debug("peer:%s begin download new index:%d", p.Identify(), index)
	d := &PeerDownload{
		index:     index,
		data:      make([]byte, p.t.PieceLength),
		beginTime: time.Now().Unix(),
	}
	for d.backlog < MaxBackLog && d.requested < p.t.PieceLength {
		need := BlockSize
		if p.t.PieceLength-d.requested < BlockSize {
			need = p.t.PieceLength - d.requested
		}
		if !p.SendRequestMsg(index, d.requested, need) {
			l4g.Error("peer:%s begin download index:%d send request msg error", p.Identify(), index)
			return false
		}
		d.requested += need
		d.backlog++
	}
	p.download = d
	return true
}

func (p *PeerClient) NewDownload(payload *PiecePayload) {
	downloaded := len(payload.Block)
	l4g.Debug("peer:%s download:%d new download", p.Identify(), downloaded)
	if p.download == nil {
		l4g.Debug("peer:%s new download has no download info", p.Identify())
		return
	}

	d := p.download
	d.backlog-- //队列要减少1
	start := payload.Begin
	end := start + downloaded
	d.downloaded += downloaded
	copy(d.data[start:end], payload.Block)
	//d.data = append(d.data, data...)
	if d.downloaded >= p.t.PieceLength {
		l4g.Debug("peer:%s new download has finish, index:%d requested:%d, piece_length:%d", p.Identify(), p.download.index, p.download.requested, p.t.PieceLength)
		totalPiecePayload := &TotalPiecePayload{
			Index: p.download.index,
			Block: p.download.data,
		}
		p.pieceChan <- totalPiecePayload
		p.download = nil
		return
	}
	for d.backlog < MaxBackLog && d.requested < p.t.PieceLength {
		need := BlockSize
		if p.t.PieceLength-d.requested < BlockSize {
			need = p.t.PieceLength - d.requested
		}
		if !p.SendRequestMsg(d.index, d.requested, need) {
			l4g.Error("peer:%d new download send request error", p.Identify())
			return
		}
		d.requested += need
		d.backlog++
	}
}

func (p *PeerClient) CheckDownload() {
	if p.download != nil {
		now := time.Now().Unix()
		if now-p.download.beginTime >= 2*60 {
			l4g.Error("peer:%s download index:%d use too much time,begin_time:%d， now:%d downloaded:%d", p.Identify(), p.download.index, p.download.beginTime, now, p.download.downloaded)
			p.workQueue <- &Work{p.download.index} //任务放回去
			p.download = nil
		}
	}
}
