package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	UDP_PORT    = ":8081"
	BUFFER_SIZE = 1024
	MAX_RETRIES = 3
	TIMEOUT     = 2 * time.Second
)

func main() {
	var mode = flag.String("mode", "", "Mode: 'server' or 'client'")
	var file = flag.String("file", "", "File to send (client mode only)")
	flag.Parse()

	switch *mode {
	case "server":
		runUDPServer()
	case "client":
		if *file == "" {
			fmt.Println("Client mode requires -file parameter")
			fmt.Println("Usage: go run udp.go -mode=client -file=path/to/file")
			os.Exit(1)
		}
		runUDPClient(*file)
	default:
		fmt.Println("Usage:")
		fmt.Println("  Server: go run udp.go -mode=server")
		fmt.Println("  Client: go run udp.go -mode=client -file=path/to/file")
		os.Exit(1)
	}
}

func runUDPServer() {
	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll("uploads", 0755); err != nil {
		fmt.Printf("Error creating uploads directory: %v\n", err)
		return
	}

	// Start UDP server
	addr, err := net.ResolveUDPAddr("udp", UDP_PORT)
	if err != nil {
		fmt.Printf("Error resolving UDP address: %v\n", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Printf("Error starting UDP server: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Printf("UDP Server listening on port %s\n", UDP_PORT)
	fmt.Println("Waiting for file transfers...")

	for {
		handleUDPFileTransfer(conn)
	}
}

func handleUDPFileTransfer(conn *net.UDPConn) {
	buffer := make([]byte, BUFFER_SIZE+20) // Extra space for headers

	// Read first packet (should contain file header)
	n, clientAddr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		fmt.Printf("Error reading from UDP: %v\n", err)
		return
	}

	fmt.Printf("New file transfer from %s\n", clientAddr.String())

	// Parse file header from first packet
	if n < 12 { // Minimum header size
		fmt.Println("Invalid header packet")
		return
	}

	filenameLen := uint32(buffer[0])<<24 | uint32(buffer[1])<<16 | uint32(buffer[2])<<8 | uint32(buffer[3])
	if filenameLen > 255 || int(filenameLen)+12 > n {
		fmt.Println("Invalid filename length")
		return
	}

	filename := string(buffer[4 : 4+filenameLen])
	fileSize := uint64(buffer[4+filenameLen])<<56 | uint64(buffer[5+filenameLen])<<48 |
		uint64(buffer[6+filenameLen])<<40 | uint64(buffer[7+filenameLen])<<32 |
		uint64(buffer[8+filenameLen])<<24 | uint64(buffer[9+filenameLen])<<16 |
		uint64(buffer[10+filenameLen])<<8 | uint64(buffer[11+filenameLen])

	fmt.Printf("Receiving file: %s (%d bytes)\n", filename, fileSize)

	// Send ACK for header
	ack := []byte("HEADER_ACK")
	_, err = conn.WriteToUDP(ack, clientAddr)
	if err != nil {
		fmt.Printf("Error sending header ACK: %v\n", err)
		return
	}

	// Create output file
	outputPath := filepath.Join("uploads", filename)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		return
	}
	defer outputFile.Close()

	// Receive file data packets
	startTime := time.Now()
	var totalReceived uint64
	expectedSeqNum := uint32(0)
	receivedPackets := make(map[uint32][]byte)

	conn.SetReadDeadline(time.Now().Add(TIMEOUT))

	for totalReceived < fileSize {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Println("Timeout waiting for data packet")
				break
			}
			fmt.Printf("Error reading data packet: %v\n", err)
			break
		}

		if n < 8 { // Minimum packet header size
			continue
		}

		// Parse packet header
		seqNum := uint32(buffer[0])<<24 | uint32(buffer[1])<<16 | uint32(buffer[2])<<8 | uint32(buffer[3])
		isLast := buffer[4] == 1
		dataSize := uint16(buffer[5])<<8 | uint16(buffer[6])

		if int(dataSize) > n-8 {
			fmt.Printf("Invalid data size in packet %d\n", seqNum)
			continue
		}

		// Store packet data
		packetData := make([]byte, dataSize)
		copy(packetData, buffer[8:8+dataSize])
		receivedPackets[seqNum] = packetData

		// Send ACK
		ackMsg := make([]byte, 4)
		ackMsg[0] = byte(seqNum >> 24)
		ackMsg[1] = byte(seqNum >> 16)
		ackMsg[2] = byte(seqNum >> 8)
		ackMsg[3] = byte(seqNum)
		_, err = conn.WriteToUDP(ackMsg, clientAddr)
		if err != nil {
			fmt.Printf("Error sending ACK for packet %d: %v\n", seqNum, err)
		}

		// Write packets in order
		for {
			if data, exists := receivedPackets[expectedSeqNum]; exists {
				_, err = outputFile.Write(data)
				if err != nil {
					fmt.Printf("Error writing to file: %v\n", err)
					return
				}
				totalReceived += uint64(len(data))
				delete(receivedPackets, expectedSeqNum)
				expectedSeqNum++

				// Progress indicator
				progress := float64(totalReceived) / float64(fileSize) * 100
				fmt.Printf("\rProgress: %.2f%% (%d/%d bytes)", progress, totalReceived, fileSize)
			} else {
				break
			}
		}

		if isLast {
			break
		}

		// Reset timeout
		conn.SetReadDeadline(time.Now().Add(TIMEOUT))
	}

	duration := time.Since(startTime)
	fmt.Printf("\nFile transfer completed in %v\n", duration)
	fmt.Printf("Average speed: %.2f KB/s\n", float64(totalReceived)/1024/duration.Seconds())
	fmt.Printf("File saved as: %s\n", outputPath)
	fmt.Printf("Received %d/%d bytes\n", totalReceived, fileSize)
	fmt.Println("---")
}

func runUDPClient(filePath string) {
	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Printf("Error accessing file: %v\n", err)
		return
	}

	// Resolve server address
	serverAddr, err := net.ResolveUDPAddr("udp", "localhost"+UDP_PORT)
	if err != nil {
		fmt.Printf("Error resolving server address: %v\n", err)
		return
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		fmt.Printf("Error connecting to server: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Printf("Connected to UDP server at localhost%s\n", UDP_PORT)

	// Open file for reading
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	filename := filepath.Base(filePath)
	fileSize := uint64(fileInfo.Size())

	fmt.Printf("Sending file: %s (%d bytes)\n", filename, fileSize)

	// Send file header
	err = sendUDPFileHeader(conn, filename, fileSize)
	if err != nil {
		fmt.Printf("Error sending file header: %v\n", err)
		return
	}

	// Send file data
	err = sendUDPFileData(conn, file, fileSize)
	if err != nil {
		fmt.Printf("Error sending file data: %v\n", err)
		return
	}

	fmt.Println("File transfer completed successfully!")
}

func sendUDPFileHeader(conn *net.UDPConn, filename string, fileSize uint64) error {
	// Create header packet
	filenameLen := uint32(len(filename))
	headerSize := 4 + filenameLen + 8 // filename_len + filename + file_size
	header := make([]byte, headerSize)

	// Pack filename length
	header[0] = byte(filenameLen >> 24)
	header[1] = byte(filenameLen >> 16)
	header[2] = byte(filenameLen >> 8)
	header[3] = byte(filenameLen)

	// Pack filename
	copy(header[4:4+filenameLen], []byte(filename))

	// Pack file size
	offset := 4 + filenameLen
	header[offset] = byte(fileSize >> 56)
	header[offset+1] = byte(fileSize >> 48)
	header[offset+2] = byte(fileSize >> 40)
	header[offset+3] = byte(fileSize >> 32)
	header[offset+4] = byte(fileSize >> 24)
	header[offset+5] = byte(fileSize >> 16)
	header[offset+6] = byte(fileSize >> 8)
	header[offset+7] = byte(fileSize)

	// Send header with retries
	for retry := 0; retry < MAX_RETRIES; retry++ {
		_, err := conn.Write(header)
		if err != nil {
			return fmt.Errorf("failed to send header: %v", err)
		}

		// Wait for ACK
		conn.SetReadDeadline(time.Now().Add(TIMEOUT))
		ackBuf := make([]byte, 20)
		n, err := conn.Read(ackBuf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Printf("Header ACK timeout, retry %d/%d\n", retry+1, MAX_RETRIES)
				continue
			}
			return fmt.Errorf("error reading header ACK: %v", err)
		}

		if n >= 10 && string(ackBuf[:10]) == "HEADER_ACK" {
			fmt.Println("Header acknowledged by server")
			return nil
		}
	}

	return fmt.Errorf("failed to receive header ACK after %d retries", MAX_RETRIES)
}

func sendUDPFileData(conn *net.UDPConn, file *os.File, fileSize uint64) error {
	startTime := time.Now()
	var totalSent uint64
	seqNum := uint32(0)
	buffer := make([]byte, BUFFER_SIZE)

	for totalSent < fileSize {
		// Read data from file
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("error reading file: %v", err)
		}

		if n == 0 {
			break
		}

		isLast := totalSent+uint64(n) >= fileSize

		// Create data packet
		packet := make([]byte, 8+n) // header + data

		// Pack sequence number
		packet[0] = byte(seqNum >> 24)
		packet[1] = byte(seqNum >> 16)
		packet[2] = byte(seqNum >> 8)
		packet[3] = byte(seqNum)

		// Pack is_last flag
		if isLast {
			packet[4] = 1
		} else {
			packet[4] = 0
		}

		// Pack data size
		packet[5] = byte(n >> 8)
		packet[6] = byte(n)
		packet[7] = 0 // Reserved byte

		// Pack data
		copy(packet[8:], buffer[:n])

		// Send packet with retries
		acked := false
		for retry := 0; retry < MAX_RETRIES && !acked; retry++ {
			_, err := conn.Write(packet)
			if err != nil {
				return fmt.Errorf("error sending packet %d: %v", seqNum, err)
			}

			// Wait for ACK
			conn.SetReadDeadline(time.Now().Add(TIMEOUT))
			ackBuf := make([]byte, 4)
			ackN, err := conn.Read(ackBuf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					fmt.Printf("Packet %d ACK timeout, retry %d/%d\n", seqNum, retry+1, MAX_RETRIES)
					continue
				}
				return fmt.Errorf("error reading ACK for packet %d: %v", seqNum, err)
			}

			if ackN == 4 {
				ackSeqNum := uint32(ackBuf[0])<<24 | uint32(ackBuf[1])<<16 | uint32(ackBuf[2])<<8 | uint32(ackBuf[3])
				if ackSeqNum == seqNum {
					acked = true
				}
			}
		}

		if !acked {
			return fmt.Errorf("failed to receive ACK for packet %d after %d retries", seqNum, MAX_RETRIES)
		}

		totalSent += uint64(n)
		seqNum++

		// Progress indicator
		progress := float64(totalSent) / float64(fileSize) * 100
		fmt.Printf("\rProgress: %.2f%% (%d/%d bytes)", progress, totalSent, fileSize)

		if isLast {
			break
		}
	}

	duration := time.Since(startTime)
	fmt.Printf("\nFile transfer completed in %v\n", duration)
	fmt.Printf("Average speed: %.2f KB/s\n", float64(totalSent)/1024/duration.Seconds())
	fmt.Printf("Sent %d packets\n", seqNum)

	return nil
}
