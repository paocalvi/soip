// small project main.go
package main

import (
	//"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CONN_HOST      = "0.0.0.0"
	CONN_PORT      = "3333"
	CONN_TYPE      = "tcp"
	ESC       byte = 27
	ESCS           = string(27)
	PROMPT         = "SC>"
)

var lastSession int = 0

func main() {
	var wg sync.WaitGroup
	//ts := new(SoipTotalStatus)
	ts := SoipTotalStatus{InternalData: make(map[string]*SoipStatus)}
	//ts.InternalData = make(map[string]SoipStatus)
	totalconfig, err := ReadTotalConfig("soip.config")
	if err != nil {
		log.Fatal("ReadConfig error ", err)
	}
	defer closeFiles(&ts)
	for i, riga := range totalconfig.InternalData {
		wg.Add(1)
		fmt.Printf("RIGA %d .. %d .. %s .. %d .. %t\n", i, riga.Port, riga.Serial, riga.Baud, riga.Master)
		onestatus := SoipStatus{BytesRead: 0, BytesWritten: 0, Active: false, Sniff: make(map[int]*WriterConnEntity), Record: nil}
		ts.setStatus(riga.getKey(), &onestatus)
		if riga.Serial == "MANAGEMENT" {
			go mngHandler(riga, ts)
		} else {
			go oneHandler(riga, wg, &onestatus)
		}
	}
	wg.Wait()
	fmt.Println("Exiting from everything")
}

func mngHandler(soipConfig SoipConfig, ts SoipTotalStatus) {
	fmt.Printf("starting management Handler on %d\n", soipConfig.Port)
	port := soipConfig.Port
	sport := strconv.FormatUint(uint64(port), 10)
	fmt.Println("Try to listen (mng) on " + CONN_HOST + ":" + sport)
	mycode := CONN_HOST + " on " + sport
	l, err := net.Listen(CONN_TYPE, CONN_HOST+":"+sport)
	if err != nil {
		fmt.Println(mycode+": Error listening:", err.Error())
		return
	}
	// Close the listener before exiting
	defer l.Close()
	for {
		// Listen for an incoming connection.
		fmt.Println("Listening on "+CONN_HOST+":"+sport, "MNG")
		conn, err := l.Accept()

		if err != nil {
			fmt.Println(mycode+": Error accepting: ", err.Error())
			return
		}
		// Handle connections in a new goroutine.
		lastSession++
		fmt.Println("New Mng session starting :", lastSession)
		go mngLine(conn, ts, lastSession)
		fmt.Println("Relistening on", port)
	}
	fmt.Println("Exiting from manage")
}

func oneHandler(soipConfig SoipConfig, wg sync.WaitGroup, status *SoipStatus) {
	defer wg.Done()
	fmt.Printf("starting Handler:%d .. %s .. %d .. %t\n", soipConfig.Port, soipConfig.Serial, soipConfig.Baud, soipConfig.Master)
	port := soipConfig.Port
	thecom := soipConfig.Serial
	sport := strconv.FormatUint(uint64(port), 10)
	baud := int(soipConfig.Baud)
	fmt.Println("Try to listen on "+CONN_HOST+":"+sport, baud)
	mycode := thecom + " on " + sport
	l, err := net.Listen(CONN_TYPE, CONN_HOST+":"+sport)
	if err != nil {
		fmt.Println(mycode+": Error listening:", err.Error())
		return
	}
	// Close the listener before exiting
	defer l.Close()
	for {
		// Listen for an incoming connection.
		fmt.Println("Listening (accept) on "+CONN_HOST+":"+sport, thecom)
		conn, err := l.Accept()
		if err != nil {
			fmt.Println(mycode+": Error accepting: ", err.Error())
			return
		}
		fmt.Printf("Incoming connection on %s from %s\n", sport, conn.RemoteAddr().String())
		// Opens the serial
		//serialConfig := &serial.Config{Name: thecom, Baud: baud, Size: soipConfig.DataBits, Parity: soipConfig.Parity, StopBits: soipConfig.StopBits, ReadTimeout: time.Second * 5}
		//serialPort, err := serial.OpenPort(serialConfig)
		serialPort, err := createAndOpen(soipConfig.IISerialConfig)
		if err != nil {
			fmt.Println(mycode+": ERRORE openPort", err.Error())
			fmt.Println("Closing port", sport)
			conn.Write([]byte("Cannot open port " + thecom + ":" + err.Error()))
			conn.Close()
			continue
		}
		// Handle connections in a new goroutine.
		quit := make(chan string)
		(*status).Active = true
		(*status).BytesRead = 0
		(*status).BytesWritten = 0
		go binaryReadFromSerialAndWriteToConnStoppable(conn, serialPort, quit, status)
		/*
		* Il fatto che questa non sia 'go' garantisce che se uno e' gia' connesso
		* ad una porta gli altri trovano occupato.
		* E' voluto in quanto mentre scrivere in due e' teoricamente accettabile,
		* non sarebbe chiaro a chi vanno le risposte.
		* Inoltre c'e' il discorso di apertura chiusura porte che andrebbe
		* gestito diversamente
		* Poi magari si puo' decidere di cambiare..
		 */
		binaryReadFromConnAndWriteToSerial(conn, serialPort, status)
		//serialPort.Flush()
		serialPort.Close()
		(*status).Active = false
		fmt.Printf("Connection %s finished: w=%d r=%d\n", soipConfig.getKey(), (*status).BytesWritten, (*status).BytesRead)
		if status.Record != nil && status.Record.TheFile != nil && status.Record.fileStatus == FOS_INUSE {
			fmt.Println("Cloing file", status.Record.TheFile.Name())
			err := status.Record.TheFile.Close()
			if err != nil {
				fmt.Println("Error closing file", err.Error())
			}
			status.Record.fileStatus = FOS_CLOSED
		}
		fmt.Println("Sending QUIT signal")
		quit <- "esci"
		fmt.Println("Recycling", port)
	}
	fmt.Println("Exiting from everything")
}

func myReader(conn net.Conn, ch *CommandHistory) (string, error) {
	_, err := conn.Write([]byte(PROMPT))
	if err != nil {
		fmt.Println("Cannot write on mng line", err.Error)
		return "", err
	}
	sb := make([]byte, 0, 1024)
	for {
		buf := make([]byte, 1024)
		// leggo
		reqLen, err := conn.Read(buf)
		if err != nil {
			// se errore, esco
			fmt.Println("Error reading TCP:", err.Error())
			fmt.Println("break")
			return "", err
		}
		fmt.Println(reqLen, "bytes read:", buf[:reqLen])
		// se ho letto empty line (o return), ritorno quanto letto sinora
		if isEmptyLine(buf[:reqLen]) {
			return string(sb), nil
		}
		// analizzo la lettura, blocco per blocco
		for i := 0; i < reqLen; {
			fmt.Println("i=", i)
			if reqLen == 1 {
				// se ho letto un solo carattere
				if buf[0] == 8 { // delete
					if len(sb) > 0 {
						conn.Write([]byte(ESCS + "[K"))
						sb = sb[:len(sb)-1]
					} else {
						conn.Write([]byte(ESCS + "[C"))
					}
					break
				}
				if buf[0] == 27 { // ESC (reset line)
					if len(sb) > 0 {
						conn.Write([]byte(fmt.Sprintf(" %s[%dD%s[K", ESCS, len(sb), ESCS)))
						sb = sb[:0]
					}
					break
				}
				if buf[0] > 31 && buf[0] != 127 {
					sb = append(sb, buf[i])
					break
				}
				fmt.Println("Invalid char : ", buf[0])
				break
			}
			// caso 'FRECCIA'
			if reqLen == 3 && buf[0] == ESC && buf[1] == '[' {
				fmt.Println("FRECCIA", buf[2], string(buf[2]))
				switch buf[2] {
				case 'D':
					{
						// cursore indietro, non gestito, lo rimando avanti
						//conn.Write([]byte(ESCS + "[C"))
						break
					}
				case 'C':
					{
						// cursore avanti, lo 'mangio'
						break
					}
				case 'A':
					{
						last := ch.goBack()
						fmt.Println("Detected last command = ", last)
						if len(last) > 0 {
							if len(sb) > 0 {
								conn.Write([]byte(fmt.Sprintf("%s[%dD%s[K", ESCS, len(sb), ESCS)))
							}
							sb = []byte(last)
							conn.Write(sb)
						}
						break
					}
				case 'B':
					{
						last := ch.goFwd()
						fmt.Println("Detected next command = ", last)
						if len(last) > 0 {
							if len(sb) > 0 {
								conn.Write([]byte(fmt.Sprintf("%s[%dD%s[K", ESCS, len(sb), ESCS)))
							}
							sb = []byte(last)
							conn.Write(sb)
						}
						break
					}
				}
				break
			}
			// regular char, appended
			if buf[i] > 31 && buf[i] != 127 {
				sb = append(sb, buf[i])
				i++
				continue
			}
			// if line terminated take input so far and discard the rest
			if isLineTerminator(buf[i:]) {
				if len(sb) == 0 {
					return "", nil
				} else {
					return string(sb), nil
				}
			}
			fmt.Println("Discarding invalid char ", buf[i])
			i++
		}
	}
}

func isEmptyLine(line []byte) bool {
	if len(line) == 0 {
		return true
	}
	if isLineTerminator(line) {
		return true
	}
	return false
}

func isLineTerminator(line []byte) bool {
	if len(line) == 1 && (line[0] == 10 || line[0] == 13) {
		return true
	}
	if len(line) >= 2 && ((line[0] == 10 && line[1] == 13) || (line[0] == 13 && line[1] == 10)) {
		return true
	}
	return false
}

func mngLine(conn net.Conn, ts SoipTotalStatus, sessionNum int) {
	fmt.Println("Enter mngLine ", sessionNum)
	defer conn.Close()
	_, err := conn.Write([]byte("Welcome to PACASOIP Management Console!\r\n"))
	if err != nil {
		fmt.Println("ERRORE Write PORT", err.Error())
		return
	}
	ch := CommandHistory{make([]string, 0), 0}
	for {
		//message, err := bufio.NewReader(conn).ReadString('\n')
		message, err := myReader(conn, &ch)
		if err != nil {
			fmt.Println("Error reading TCP:", err.Error())
			break
		}
		fmt.Println("message len = ", len(message))
		message = strings.TrimSpace(message)
		message = strings.Replace(message, "\r", "", -1)
		message = strings.Replace(message, "\n", "", -1)
		message = strings.Replace(message, "\t", " ", -1)
		message = strings.TrimSpace(message)
		fmt.Println("message len = ", len(message))
		if len(message) == 0 {
			continue
		}
		fmt.Println("data from reader = ", message)
		ch.addToHistory(message)
		parti := strings.Split(message, " ")
		if len(parti) == 0 {
			continue
		}
		command := strings.ToUpper(parti[0])
		n := 0
		fmt.Printf("WHAT=<%s> LEN = %d PARTS = %d\n", command, len(command), len(parti))
	SW:
		switch {
		case strings.HasPrefix("LIST", command):
			for kk, ss := range ts.InternalData {
				if !strings.HasSuffix(kk, "MANAGEMENT") {
					sniffed := ""

					if _, ok := ss.Sniff[sessionNum]; ok {
						sniffed = "(sniffed)"
					}
					linea := fmt.Sprintf("%s : %d bytes written, %d bytes read, %t %s", kk, ss.BytesWritten, ss.BytesRead, ss.Active, sniffed)
					n, err = conn.Write([]byte(linea + "\r\n"))
				}
			}
		case strings.HasPrefix("EXIT", command):
			return
		case strings.HasPrefix("SNIFF", command):
			if len(parti) > 1 {
				for kk, ss := range ts.InternalData {
					if strings.HasPrefix(kk, parti[1]+":") || strings.HasSuffix(kk, ":"+parti[1]) {
						wce := WriterConnEntity{WriterData{NOTHING}, &conn}
						ss.Sniff[sessionNum] = &wce
						linea := fmt.Sprintf("Sniff set on " + kk)
						n, err = conn.Write([]byte(linea + "\r\n"))
					} else {
						fmt.Println(kk, "<>", parti[1])
						delete(ss.Sniff, sessionNum)
					}
				}
			} else {
				fmt.Println("Missing connection name")
			}
		case strings.HasPrefix("FILE", command):
			if len(parti) == 3 {
				fn := parti[2]
				for kk, ss := range ts.InternalData {
					if strings.HasPrefix(kk, parti[1]+":") || strings.HasSuffix(kk, ":"+parti[1]) {
						if ss.Record != nil && ss.Record.TheFile != nil {
							fmt.Println(kk, "already registered!")
							conn.Write([]byte(kk + " already registered on " + ss.Record.TheFile.Name()))
							break SW
						}
						theFile, err := os.Create(fn)
						if err != nil {
							fmt.Println("Cannot create ", fn, err.Error())
							conn.Write([]byte("Cannot create file " + fn + "\r\n"))
							break SW
						}
						wcf := WriterFileEntity{WriterData{NOTHING}, theFile, FOS_INUSE}
						ss.Record = &wcf
						linea := fmt.Sprintf("File recording set on %s -> %s\r\n", kk, fn)
						n, err = conn.Write([]byte(linea))
					}
				}
			} else {
				conn.Write([]byte("Usage: FILE <Port or Serial> <Filename>\r\n"))
				fmt.Println("Usage: FILE <Port or Serial> <Filename>")
			}
		default:
			n, err = conn.Write([]byte("Unknown command:" + command + "\r\n"))
		}
		if err != nil {
			fmt.Println("ERRORE Write PORT", err.Error())
			return
		}
		fmt.Println("Bytes writtes to conn=", n)
	}
	fmt.Println("Exiting mngLine")
}

//func binaryReadFromConnAndWriteToSerial(conn net.Conn, serialPort *serial.Port, status *SoipStatus) {
func binaryReadFromConnAndWriteToSerial(conn io.Reader, serialPort io.Writer, status *SoipStatus) {
	fmt.Println("Enter binaryReadFromConnAndWriteToSerial")
	for {
		buf := make([]byte, 1024)
		reqLen, err := conn.Read(buf)
		if err != nil {
			fmt.Println("Error reading TCP:", err.Error())
			fmt.Println("break")
			break
		}
		fmt.Println("data from conn = ", reqLen)
		writeOnManageLineOrFile(status, buf[:reqLen], TX)
		n, err := serialPort.Write(buf[:reqLen])
		if err != nil {
			fmt.Println("Error writing on serial:", err.Error())
			fmt.Println("break")
			break
		}
		(*status).activity(reqLen, 0)
		fmt.Println("Bytes written to serial=", n)
	}
	fmt.Println("Exiting binaryReadFromConnAndWriteToSerial")
	//conn.Close()
}

//func binaryReadFromSerialAndWriteToConnStoppable(conn net.Conn, serialPort *serial.Port, quit chan string, status *SoipStatus) {
func binaryReadFromSerialAndWriteToConnStoppable(conn io.Writer, serialPort io.Reader, quit chan string, status *SoipStatus) {
	fmt.Println("Enter binaryReadFromSerialAndWriteToConnStoppable")
	for {
		select {
		case <-quit:
			fmt.Println("QUIT RECEIVED, exiting binaryReadFromSerialAndWriteToConnStoppable")
			return
		default:
			buf := make([]byte, 1024)
			//fmt.Println("Reading serial....")
			reqLen, err := serialPort.Read(buf)
			//fmt.Println("ECCO")
			if err != nil {
				fmt.Println("Error reading SERIAL:", err.Error())
				fmt.Println("exit select because of SERIAL (read) ERROR")
				break
			}
			if reqLen > 0 {
				fmt.Println("data from serial = ", reqLen)
				writeOnManageLineOrFile(status, buf[:reqLen], RX)
				n, err := conn.Write(buf[:reqLen])
				if err != nil {
					fmt.Println("Error writing TCP : ", err.Error())
					fmt.Println("exit select because of TCP (write) ERROR")
					break
					//log.Fatal(err)
				}
				(*status).activity(0, reqLen)
				fmt.Println("Bytes writtes to conn=", n)
			}
		}

	}
	fmt.Println("Exiting binaryReadFromSerialAndWriteToConnStoppable")
}

func writeOnManageLineOrFile(status *SoipStatus, toWrite []byte, dir WriterStatus) {
	if len(status.Sniff) > 0 {
		for k, w := range status.Sniff {
			fmt.Println("Sniffed by ", k)
			_, err := writeOnConn(*w.TheConn, toWrite, dir, w.WriterData.LastPublished)
			if err != nil {
				fmt.Println("Error writing on manage line:", k, err.Error())
			} else {
				w.WriterData.LastPublished = dir
			}
		}
	}
	if status.Record != nil && status.Record != nil {
		fmt.Println("Recorded by ", status.Record.TheFile.Name())
		manageRecordingFileStatus(status)
		if status.Record.fileStatus == FOS_INUSE {
			_, err := writeOnConn(status.Record.TheFile, toWrite, dir, status.Record.WriterData.LastPublished)
			if err != nil {
				fmt.Println("Error writing on file:", status.Record.TheFile.Name(), err.Error())
			} else {
				status.Record.TheFile.Sync()
				status.Record.WriterData.LastPublished = dir
			}
		}
	}
}

func manageRecordingFileStatus(ss *SoipStatus) {
	switch ss.Record.fileStatus {
	case FOS_INUSE:
		// Nothing to do, file already in use and opened
	case FOS_CLOSED:
		newFile, err := os.OpenFile(ss.Record.TheFile.Name(), os.O_APPEND|os.O_RDWR, 0666)
		if err != nil {
			fmt.Println("Error in reopening file ", ss.Record.TheFile.Name(), err.Error())
		} else {
			ss.Record.TheFile = newFile
			ss.Record.fileStatus = FOS_INUSE
		}
	}
}

func writeOnConn(conn io.Writer, what []byte, direction WriterStatus, lastDirection WriterStatus) (int, error) {
	displacement := ""
	if direction == RX {
		displacement = string(createByteArrayOfSpaces(40))
	}
	if direction != lastDirection {
		clause := "===TO===>"
		if direction == RX {
			clause = "<==FROM=="
		}
		line := fmt.Sprint("\r\n", displacement, "Data ", clause, " SERIAL")
		_, err := conn.Write([]byte(line))
		if err != nil {
			fmt.Println("Error Writing", err.Error())
			return -1, err
		}
	}
	line := fmt.Sprint("\r\n", displacement, "[", time.Now().Format("2006-01-02 15:04:05.999999"), "]\r\n")
	_, err := conn.Write([]byte(line))
	if err != nil {
		fmt.Println("Error Writing ", err.Error())
		return -1, err
	}
	for i := 0; i < 1+len(what)/8; i++ {
		var pezzo []byte
		if i*8+7 > len(what) {
			pezzo = what[i*8:]
		} else {
			pezzo = what[i*8 : i*8+8]
		}
		_, err := conn.Write([]byte(displacement + format(pezzo) + "\r\n"))
		if err != nil {
			fmt.Println("Error Writing ", err.Error())
			return -1, err
		}
	}
	return len(what), nil

}

func formatHexWithSpaces(content []byte) string {
	var result []byte
	for i := 0; i < len(content); i += 2 {
		result = append(result, content[i], content[i+1], 32)
	}
	return string(result)
}

func translateInAscii(content []byte) string {
	var result []byte
	for i := 0; i < len(content); i++ {
		b := content[i]
		if b > 31 && b < 128 {
			result = append(result, b)
		} else {
			result = append(result, '.')
		}
	}
	return string(result)
}

func format(content []byte) string {
	ret := hex.EncodeToString(content)
	ret = strings.ToUpper(ret)
	fmt.Println("ret=", ret)
	separation := createByteArrayOfSpaces((8-len(content))*3 + 4)
	ret = formatHexWithSpaces([]byte(ret)) + string(separation) + translateInAscii(content)
	return ret
}

func createByteArrayOfSpaces(howmany int) []byte {
	var result []byte
	for i := 0; i < howmany; i++ {
		result = append(result, ' ')
	}
	return result
}

func closeFiles(tc *SoipTotalStatus) {
	fmt.Println("Closing")
	for _, v := range tc.InternalData {
		if v.Record != nil && v.Record.TheFile != nil {
			fmt.Println("Closing ", v.Record.TheFile.Name())
			err := v.Record.TheFile.Close()
			if err != nil {
				fmt.Println("Cannot close ", v.Record.TheFile.Name(), err.Error())
			}
		}
	}
}
