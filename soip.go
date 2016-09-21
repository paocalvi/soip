// small project main.go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"
)

const (
	CONN_HOST = "0.0.0.0"
	CONN_PORT = "3333"
	CONN_TYPE = "tcp"
)

func main() {
	// Listen for incoming connections.
	var wg sync.WaitGroup
	//ts := new(SoipTotalStatus)
	ts := SoipTotalStatus{InternalData: make(map[string]*SoipStatus)}
	//ts.InternalData = make(map[string]SoipStatus)
	totalconfig, err := ReadTotalConfig("soip.config")
	if err != nil {
		log.Fatal("ReadConfig error ", err)
	}
	for i, riga := range totalconfig.InternalData {
		wg.Add(1)
		fmt.Printf("RIGA %d .. %d .. %s .. %d .. %t\n", i, riga.Port, riga.Serial, riga.Baud, riga.Master)
		onestatus := SoipStatus{BytesRead: 0, BytesWritten: 0, Active: false}
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
		go mngLine(conn, ts)
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
		serialConfig := &serial.Config{Name: thecom, Baud: baud, Size: soipConfig.DataBits, Parity: soipConfig.Parity, StopBits: soipConfig.StopBits, ReadTimeout: time.Second * 5}
		serialPort, err := serial.OpenPort(serialConfig)
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
		serialPort.Flush()
		serialPort.Close()
		(*status).Active = false
		fmt.Printf("Connection %s finished: w=%d r=%d\n", soipConfig.getKey(), (*status).BytesWritten, (*status).BytesRead)
		fmt.Println("Sending QUIT signal")
		quit <- "esci"
		fmt.Println("Recycling", port)
	}
	fmt.Println("Exiting from everything")
}

func mngLine(conn net.Conn, ts SoipTotalStatus) {
	fmt.Println("Enter mngLine")
	_, err := conn.Write([]byte("Welcome to PACASOIP Management Console!\r\n"))
	if err != nil {
		fmt.Println("ERRORE Write PORT")
		log.Fatal(err)
	}
	for {
		message, err := bufio.NewReader(conn).ReadString('\n')
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
		fmt.Println("data from conn = ", message)
		parti := strings.Split(message, " ")
		if len(parti) == 0 {
			continue
		}
		command := strings.ToUpper(parti[0])
		n := 0
		fmt.Printf("WHAT=<%s> LEN = %d PARTS = %d\n", command, len(command), len(parti))
		switch {
		case strings.HasPrefix("LIST", command):
			for kk, ss := range ts.InternalData {
				if !strings.HasSuffix(kk, "MANAGEMENT") {
					linea := fmt.Sprintf("%s : %d bytes written, %d bytes read, %t", kk, ss.BytesWritten, ss.BytesRead, ss.Active)
					n, err = conn.Write([]byte(linea + "\r\n"))
				}
			}
			fmt.Println("LIST")
		default:
			n, err = conn.Write([]byte("Unknown command:" + command + "\r\n"))
		}
		if err != nil {
			fmt.Println("ERRORE Write PORT")
			log.Fatal(err)
		}
		fmt.Println("Bytes writtes to conn=", n)
	}
	fmt.Println("Exiting mngLine")
	conn.Close()
}

func binaryReadFromConnAndWriteToSerial(conn net.Conn, serialPort *serial.Port, status *SoipStatus) {
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

func binaryReadFromSerialAndWriteToConnStoppable(conn net.Conn, serialPort *serial.Port, quit chan string, status *SoipStatus) {
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
