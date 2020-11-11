package main

import (
	"app/client"
	"app/torrent"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"

	l4g "github.com/ivanabc/log4go"
)

var gSrc = flag.String("src", "../torrent/test.torrent", "目标地址")
var gDst = flag.String("dst", "../torrent/", "保存地址")

var gPieceChan = make(chan *client.PiecePayload, 100)
var gWorkQueue chan *client.Work

func main() {

	flag.Parse()

	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()

	torr, err := torrent.OpenTorrent(*gSrc)
	if err != nil {
		log.Fatal(fmt.Sprintf("parse torrent parse file error %s", err))
	}
	newTorr := torrent.ChangeBencodeToTorrent(torr)
	if newTorr == nil {
		log.Fatal(fmt.Sprintf("change torrent file error"))
	}
	info, err := newTorr.GetTrackInfo()
	if err != nil {
		log.Fatal(fmt.Sprintf("get tracker info error %s", err))
	}
	fmt.Printf("get info %+v \n", info)
	newInfo := torrent.ChangeBencodeToTrack(info)
	fmt.Printf("get newInfo %+v \n", newInfo)
	pieceCount := newTorr.Length / newTorr.PieceLength
	if newTorr.Length%newTorr.PieceLength != 0 {
		pieceCount++
	}

	var wg sync.WaitGroup
	bgContext := context.Background()
	childContext, cancel := context.WithCancel(bgContext)
	gWorkQueue = make(chan *client.Work, pieceCount)
	peerId := 1
	for _, v := range newInfo.Peers {
		client := client.NewPeerClient(peerId, v, newTorr, gPieceChan)
		wg.Add(1)
		go client.Run(childContext, &wg, gWorkQueue)
		peerId++
	}
	l4g.Info("begin download:%s total length:%d total piece:%d, per piece length:%d", newTorr.Name, newTorr.Length, pieceCount, newTorr.PieceLength)
	for i := 0; i < pieceCount; i++ {
		gWorkQueue <- &client.Work{i}
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)

	totalLen := 0
	receiveData := make([]byte, newTorr.Length)
	for {
		select {
		case piecePayload := <-gPieceChan:
			l4g.Debug("main receive data %d %d", piecePayload.Index, piecePayload.Begin)
			totalLen += len(piecePayload.Block)
			begin := piecePayload.Index*newTorr.PieceLength + piecePayload.Begin
			end := begin + len(piecePayload.Block)
			copy(receiveData[begin:end], piecePayload.Block)
			l4g.Info("download data :%d now:%d total:%d", int(float64(totalLen)/float64(newTorr.Length)*10000.0), totalLen, newTorr.Length)
			if totalLen >= newTorr.Length {
				newTorr.Save(receiveData, *gDst)
				l4g.Debug("main receive total data %d", totalLen)
				cancel()
				goto FINISH
			}
		case sig := <-sigs:
			l4g.Debug("main receive sig %d", sig)
			cancel()
			goto FINISH
		}
	}

FINISH:
	l4g.Debug("main into Finish")
	wg.Wait()
	l4g.Info("main end")
}
