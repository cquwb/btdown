package download

import (
	"app/torrent"
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	l4g "github.com/ivanabc/log4go"
	bencode "github.com/jackpal/bencode-go"
)

type DownloadManager struct {
	ManagerFile   *os.File
	SaveFile      *os.File
	PieceLength   int
	Total         int
	Finished      int
	FinishedIndex []int
}

type DownloadEncode struct {
	Total       int   `bencode:"total"`
	Finish      int   `bencode:"finish"`
	FinishIndex []int `bencode:"finish_index"`
}

func (dm *DownloadManager) GetFinished() []int {
	return dm.FinishedIndex
}

func (dm *DownloadManager) GetIsFinished() bool {
	return dm.Finished >= dm.Total
}

//返回是否全部完成
func (dm *DownloadManager) SetFinished(index int, data []byte) bool {
	start := int64(index) * int64(dm.PieceLength)
	dm.SaveFile.Seek(int64(start), 0)
	dm.Finished++
	dm.FinishedIndex = append(dm.FinishedIndex, index)
	dm.SaveFile.Write(data)
	return dm.GetIsFinished()
}

func NewDownloadManager(t *torrent.Torrent, path string) (*DownloadManager, error) {
	managerPath := filepath.Join(path, t.Name)
	managerPath += "_tmptmp"
	newManager, managerFile, err := openFile(managerPath, 0)
	if err != nil {
		l4g.Error("download manager open manager file:%s error:%s", managerPath, err)
		return nil, err
	}

	savePath := filepath.Join(path, t.Name)
	_, saveFile, err := openFile(savePath, int64(t.Length))
	if err != nil {
		l4g.Error("download manager open save file:%s error:%s", savePath, err)
		return nil, err
	}

	bencodeM := &DownloadEncode{}
	if !newManager {
		if err := bencode.Unmarshal(managerFile, bencodeM); err != nil {
			l4g.Error("download manager unmarshal benconde error:%s", err)
			return nil, err
		}
	}

	total := len(t.PiecesHash)
	if bencodeM.Total != total {
		bencodeM.Total = total
	}
	dm := &DownloadManager{
		ManagerFile:   managerFile,
		SaveFile:      saveFile,
		PieceLength:   t.PieceLength,
		Total:         bencodeM.Total,
		Finished:      bencodeM.Finish,
		FinishedIndex: bencodeM.FinishIndex,
	}
	return dm, nil
}

func openFile(path string, size int64) (bool, *os.File, error) {
	_, err := os.Stat(path)
	if err == nil { //文件存在
		file, err2 := os.OpenFile(path, os.O_RDWR, 0666)
		return false, file, err2
	} else if os.IsNotExist(err) {
		file, err2 := os.Create(path)
		if err2 != nil {
			return false, nil, err
		}
		if size > 0 {
			file.Seek(size-1, 0)
			file.Write([]byte{0})
		}
		return true, file, err2
	} else {
		fmt.Println(err)
		return false, nil, err
	}
}

func (dm *DownloadManager) Close() {
	encode := DownloadEncode{
		Total:       dm.Total,
		Finish:      dm.Finished,
		FinishIndex: dm.FinishedIndex,
	}
	l4g.Debug("download manager close save manager file encode %+v", encode)
	dm.ManagerFile.Seek(0, 0) //写到文件开头
	var data bytes.Buffer
	bencode.Marshal(&data, encode)
	if _, err := dm.ManagerFile.Write(data.Bytes()); err != nil {
		l4g.Error("download manager save manager file error %s", err)
	}
	dm.ManagerFile.Close()
	dm.SaveFile.Close()

}
func (db *DownloadManager) Delete() {
}
