package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

type IIStopBits byte
type IIParity byte
type FileOpeningStatus byte

const (
	IIStop1     IIStopBits = iota
	IIStop1Half IIStopBits = iota
	IIStop2     IIStopBits = iota
)

const (
	IIParityNone  IIParity = iota
	IIParityOdd   IIParity = iota
	IIParityEven  IIParity = iota
	IIParityMark  IIParity = iota // parity bit is always 1
	IIParitySpace IIParity = iota // parity bit is always 0
)

const (
	FOS_INUSE  FileOpeningStatus = iota
	FOS_CLOSED FileOpeningStatus = iota
)

type WriterStatus int

type WriterData struct {
	LastPublished WriterStatus
}

type WriterFileEntity struct {
	WriterData
	TheFile    *os.File
	fileStatus FileOpeningStatus
}

type WriterConnEntity struct {
	WriterData
	TheConn *net.Conn
}

const (
	NOTHING WriterStatus = iota
	TX      WriterStatus = iota
	RX      WriterStatus = iota
)

type IISerialConfig struct {
	Serial   string
	Baud     uint32
	DataBits byte
	Parity   IIParity
	StopBits IIStopBits
}

type SoipConfig struct {
	Port uint16
	IISerialConfig
	Master bool
}

type SoipTotalConfig struct {
	InternalData map[uint16]SoipConfig
}

type SoipStatus struct {
	mu           sync.Mutex
	BytesWritten uint64
	BytesRead    uint64
	Active       bool
	Sniff        map[int]*WriterConnEntity
	Record       *WriterFileEntity
}

type SoipTotalStatus struct {
	InternalData map[string]*SoipStatus
}

func readConfig(path string) (config []SoipConfig, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	for _, linea := range lines {
		fmt.Println(linea)
		pezzi := strings.Split(linea, ",")
		if len(pezzi) != 5 {
			fmt.Println("Wrong line, skipping :", linea)
			continue
		}
		port, err := strconv.ParseInt(pezzi[0], 10, 16)
		if err != nil {
			return nil, err
		}
		port16 := uint16(port)
		serialname := pezzi[1]
		speed, err := strconv.ParseInt(pezzi[2], 10, 32)
		options := pezzi[3]
		if len(options) != 3 {
			fmt.Println("Wrong options, using 8N1:", options)
			options = "8N1"
		}
		if err != nil {
			return nil, err
		}
		speed32 := uint32(speed)
		master, err := strconv.ParseBool(pezzi[4])
		if err != nil {
			return nil, err
		}
		datasize := byte(8)
		switch options[0] {
		case '7':
			datasize = byte(7)
		case '6':
			datasize = byte(6)
		case '5':
			datasize = byte(5)
		}
		stopbits := IIStop1
		switch options[2] {
		case '2':
			stopbits = IIStop2
		}
		parity := IIParityNone
		switch options[1] {
		case 'O':
			parity = IIParityOdd
		case 'E':
			parity = IIParityEven
		}
		uno := SoipConfig{Port: port16, IISerialConfig: IISerialConfig{Serial: serialname, Baud: speed32, DataBits: datasize, Parity: parity, StopBits: stopbits}, Master: master}
		config = append(config, uno)
	}
	return config, nil
}

func ReadTotalConfig(path string) (soipTotalConfig SoipTotalConfig, err error) {
	content, err := readConfig(path)
	if err != nil {
		return soipTotalConfig, err
	}
	soipTotalConfig.InternalData = make(map[uint16]SoipConfig)
	for _, riga := range content {
		soipTotalConfig.InternalData[riga.Port] = riga
	}
	return soipTotalConfig, err
}

/*
func (s *SoipTotalConfig) getConfigForPort(port uint16) (result SoipConfig, ok bool) {
	result, ok = s.InternalData[port]
	return result, ok
}
*/

func (sc SoipConfig) getKey() (result string) {
	result = strconv.FormatInt(int64(sc.Port), 10) + ":" + sc.Serial
	return result
}

func (ss *SoipStatus) activity(written int, read int) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.BytesWritten += uint64(written)
	ss.BytesRead += uint64(read)
}

func (sts SoipTotalStatus) getStatus(key string) (result *SoipStatus, ok bool) {
	result, ok = sts.InternalData[key]
	return result, ok
}

func (sts *SoipTotalStatus) setStatus(key string, payload *SoipStatus) {
	sts.InternalData[key] = payload
}
