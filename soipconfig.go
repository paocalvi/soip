package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/tarm/serial"
)

type SoipConfig struct {
	Port     uint16
	Serial   string
	Baud     uint32
	DataBits byte
	Parity   serial.Parity
	StopBits serial.StopBits
	Master   bool
}

type SoipTotalConfig struct {
	InternalData map[uint16]SoipConfig
}

type SoipStatus struct {
	mu           sync.Mutex
	BytesWritten uint64
	BytesRead    uint64
	Active       bool
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
		stopbits := serial.Stop1
		switch options[2] {
		case '2':
			stopbits = serial.Stop2
		}
		parity := serial.ParityNone
		switch options[1] {
		case 'O':
			parity = serial.ParityOdd
		case 'E':
			parity = serial.ParityEven
		}
		uno := SoipConfig{Port: port16, Serial: serialname, Baud: speed32, DataBits: datasize, Parity: parity, StopBits: stopbits, Master: master}
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

func (s *SoipTotalConfig) getConfigForPort(port uint16) (result SoipConfig, ok bool) {
	result, ok = s.InternalData[port]
	return result, ok
}

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
