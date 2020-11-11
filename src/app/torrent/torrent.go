package torrent

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	bencode "github.com/jackpal/bencode-go"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	l4g "github.com/ivanabc/log4go"
)

const (
	MyPeerId string = "sdfdsfsd"
)

const (
	Port uint16 = 6881
)

type BencodeTorrent struct {
	Announce string      `bencode:"announce"`
	Info     BencodeInfo `bencode:"info"`
}

type BencodeInfo struct {
	Pieces      string `bencode:"pieces"`       //这里是所有碎片的一个sha-1信息拼接成的一个字符串。
	PieceLength int    `bencode:"piece length"` //每个piece的长度
	Length      int    `bencode:"length"`       //整个文件的长度
	Name        string `bencode:"name"`         //文件名称
}

//对TorrentFile的清洗。
type Torrent struct {
	Announce    string
	Length      int
	PieceLength int
	Name        string
	PiecesHash  [][20]byte //每个piece的sha1.Sum()
	InfoHash    [20]byte   //整个的bencodeInfo的sha1.Sum()
	PeerId      [20]byte   //自己的
}

func OpenTorrent(path string) (*BencodeTorrent, error) {
	file, err := os.Open(path)
	if err != nil {
		l4g.Error("OpenTorretn %s error :%s", path, err)
		return nil, err
	}
	defer file.Close()
	torrent := &BencodeTorrent{}
	if err := bencode.Unmarshal(file, torrent); err != nil {
		l4g.Error("OpenTorret %s unmarshal error :%s", path, err)
		return nil, err
	}

	fmt.Printf("torrent is %+v", torrent)
	return torrent, nil
}

func ChangeBencodeToTorrent(t *BencodeTorrent) *Torrent {
	newT := &Torrent{
		Announce:    t.Announce,
		Length:      t.Info.Length,
		PieceLength: t.Info.PieceLength,
		Name:        t.Info.Name,
	}

	var peerID [20]byte
	_, err := rand.Read(peerID[:])
	if err != nil {
		l4g.Error("ChangeBencodeToTorrent error :%s", err)
		return nil
	}
	newT.PeerId = peerID

	var info bytes.Buffer
	if err := bencode.Marshal(&info, t.Info); err != nil {
		l4g.Error("ChangeBencodeToTorrent error :%s", err)
		return nil
	}
	hashLen := 20
	pieces := []byte(t.Info.Pieces)
	if len(pieces)%hashLen != 0 {
		l4g.Error("ChangeBencodeToTorrent pieces has len error :%s", err)
		return nil
	}
	hashNum := len(pieces) / hashLen
	hashInfo := make([][20]byte, hashNum)
	for i := 0; i < hashNum; i++ {
		begin := i * hashLen
		copy(hashInfo[i][:], pieces[begin:begin+hashLen])
	}
	newT.PiecesHash = hashInfo
	newT.InfoHash = sha1.Sum(info.Bytes())
	return newT
}

func (t *Torrent) GetTrackInfo() (*BencodeTrackerInfo, error) {
	serverUrl, err := t.buildTrackerURL(6881)
	if err != nil {
		l4g.Error("GetTrackInfo error %s", err)
		return nil, err
	}
	l4g.Debug("GetTrackInfo url is %s", serverUrl)
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(serverUrl)
	if err != nil {
		l4g.Error("GetTrackInfo http get error %s", err)
		return nil, err
	}
	defer resp.Body.Close()
	/*
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			l4g.Error("GetTrackInfo read body error %s", err)
			return nil, err
		}
	*/
	trackRes := &BencodeTrackerInfo{}
	if err := bencode.Unmarshal(resp.Body, trackRes); err != nil {
		l4g.Error("GetTrackInfo unmarshal body:%+v error:%s", resp.Body, err)
		return nil, err
	}
	return trackRes, nil
}

func (t *Torrent) buildTrackerURL(port uint16) (string, error) {
	base, err := url.Parse(t.Announce)
	if err != nil {
		l4g.Error("buildTrackerURL %s error %s", t.Announce, err)
		return "", err
	}
	params := url.Values{
		"info_hash":  []string{string(t.InfoHash[:])},
		"peer_id":    []string{string(t.PeerId[:])},
		"port":       []string{strconv.Itoa(int(port))},
		"downloaded": []string{"0"},
		"uploaded":   []string{"0"},
		"left":       []string{strconv.Itoa(t.Length)},
		"compact":    []string{"1"},
	}
	base.RawQuery = params.Encode()
	return base.String(), nil
}

func (t *Torrent) Save(data []byte, path string) {
	newPath := filepath.Join(path, t.Name)
	if err := ioutil.WriteFile(newPath, data, 0644); err != nil {
		l4g.Error("torrent save error:%s", err)
	}
}

type BencodeTrackerInfo struct {
	Time  uint32 `bencode:"interval"`
	Peers string `bencode:"peers"` //这个必须是一个字符串
}

type TrackerInfo struct {
	Time  uint32
	Peers []*Peer
}

type Peer struct {
	ip   net.IP
	port uint16
}

func (p *Peer) Addr() string {
	return p.ip.String() + ":" + strconv.Itoa(int(p.port))
}

func ChangeBencodeToTrack(t *BencodeTrackerInfo) *TrackerInfo {
	newT := &TrackerInfo{}
	newT.Time = t.Time
	peers := []byte(t.Peers)
	count := len(peers) / 6
	for i := 0; i < count; i++ {
		start := i * 6
		peer := Peer{
			ip:   peers[start : start+4],
			port: binary.BigEndian.Uint16(peers[start+4 : start+6]),
		}
		newT.Peers = append(newT.Peers, &peer)
	}
	return newT
}
