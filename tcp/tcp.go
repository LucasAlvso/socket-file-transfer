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
	TCP_PORT    = ":8080"
	BUFFER_SIZE = 4096
)

func main() {
	var mode = flag.String("mode", "", "Mode: 'server' or 'client'")
	var file = flag.String("file", "", "File to send (client mode only)")
	flag.Parse()

	switch *mode {
	case "server":
		runTCPServer()
	case "client":
		if *file == "" {
			fmt.Println("Client mode requires -file parameter")
			fmt.Println("Usage: go run tcp.go -mode=client -file=path/to/file")
			os.Exit(1)
		}
		runTCPClient(*file)
	default:
		fmt.Println("Usage:")
		fmt.Println("  Server: go run tcp.go -mode=server")
		fmt.Println("  Client: go run tcp.go -mode=client -file=path/to/file")
		os.Exit(1)
	}
}

func runTCPServer() {
	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll("uploads", 0755); err != nil {
		fmt.Printf("Error creating uploads directory: %v\n", err)
		return
	}

	// Start listening on TCP port
	listener, err := net.Listen("tcp", TCP_PORT)
	if err != nil {
		fmt.Printf("Error starting TCP server: %v\n", err)
		return
	}
	defer listener.Close()

	fmt.Printf("TCP Server listening on port %s\n", TCP_PORT)
	fmt.Println("Waiting for connections...")

	for {
		// Accept incoming connections
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}

		// Handle each connection in a separate goroutine
		go handleTCPConnection(conn)
	}
}

func handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	fmt.Printf("New connection from %s\n", clientAddr)

	// Read filename length first
	filenameLenBuf := make([]byte, 4)
	_, err := io.ReadFull(conn, filenameLenBuf)
	if err != nil {
		fmt.Printf("Error reading filename length: %v\n", err)
		return
	}

	filenameLen := int(filenameLenBuf[0])<<24 | int(filenameLenBuf[1])<<16 | int(filenameLenBuf[2])<<8 | int(filenameLenBuf[3])

	// Read filename
	filenameBuf := make([]byte, filenameLen)
	_, err = io.ReadFull(conn, filenameBuf)
	if err != nil {
		fmt.Printf("Error reading filename: %v\n", err)
		return
	}

	filename := string(filenameBuf)
	fmt.Printf("Receiving file: %s from %s\n", filename, clientAddr)

	// Read file size
	fileSizeBuf := make([]byte, 8)
	_, err = io.ReadFull(conn, fileSizeBuf)
	if err != nil {
		fmt.Printf("Error reading file size: %v\n", err)
		return
	}

	fileSize := int64(fileSizeBuf[0])<<56 | int64(fileSizeBuf[1])<<48 | int64(fileSizeBuf[2])<<40 | int64(fileSizeBuf[3])<<32 |
		int64(fileSizeBuf[4])<<24 | int64(fileSizeBuf[5])<<16 | int64(fileSizeBuf[6])<<8 | int64(fileSizeBuf[7])

	fmt.Printf("File size: %d bytes\n", fileSize)

	// Create output file
	outputPath := filepath.Join("uploads", filename)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		return
	}
	defer outputFile.Close()

	// Receive file data
	startTime := time.Now()
	var totalReceived int64
	buffer := make([]byte, BUFFER_SIZE)

	for totalReceived < fileSize {
		remaining := fileSize - totalReceived
		readSize := int64(BUFFER_SIZE)
		if remaining < readSize {
			readSize = remaining
		}

		n, err := conn.Read(buffer[:readSize])
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("Error reading data: %v\n", err)
			return
		}

		_, err = outputFile.Write(buffer[:n])
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
			return
		}

		totalReceived += int64(n)

		// Progress indicator
		progress := float64(totalReceived) / float64(fileSize) * 100
		fmt.Printf("\rProgress: %.2f%% (%d/%d bytes)", progress, totalReceived, fileSize)
	}

	duration := time.Since(startTime)
	fmt.Printf("\nFile transfer completed in %v\n", duration)
	fmt.Printf("Average speed: %.2f KB/s\n", float64(totalReceived)/1024/duration.Seconds())
	fmt.Printf("File saved as: %s\n", outputPath)
	fmt.Println("---")
}

func runTCPClient(filePath string) {
	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Printf("Error accessing file: %v\n", err)
		return
	}

	// Connect to server
	conn, err := net.Dial("tcp", "localhost"+TCP_PORT)
	if err != nil {
		fmt.Printf("Error connecting to server: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Printf("Connected to TCP server at localhost%s\n", TCP_PORT)

	// Open file for reading
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	filename := filepath.Base(filePath)
	fileSize := fileInfo.Size()

	fmt.Printf("Sending file: %s (%d bytes)\n", filename, fileSize)

	// Send filename length (4 bytes)
	filenameLen := len(filename)
	filenameLenBuf := []byte{
		byte(filenameLen >> 24),
		byte(filenameLen >> 16),
		byte(filenameLen >> 8),
		byte(filenameLen),
	}
	_, err = conn.Write(filenameLenBuf)
	if err != nil {
		fmt.Printf("Error sending filename length: %v\n", err)
		return
	}

	// Send filename
	_, err = conn.Write([]byte(filename))
	if err != nil {
		fmt.Printf("Error sending filename: %v\n", err)
		return
	}

	// Send file size (8 bytes)
	fileSizeBuf := []byte{
		byte(fileSize >> 56),
		byte(fileSize >> 48),
		byte(fileSize >> 40),
		byte(fileSize >> 32),
		byte(fileSize >> 24),
		byte(fileSize >> 16),
		byte(fileSize >> 8),
		byte(fileSize),
	}
	_, err = conn.Write(fileSizeBuf)
	if err != nil {
		fmt.Printf("Error sending file size: %v\n", err)
		return
	}

	// Send file data
	startTime := time.Now()
	var totalSent int64
	buffer := make([]byte, BUFFER_SIZE)

	for {
		n, err := file.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("Error reading file: %v\n", err)
			return
		}

		_, err = conn.Write(buffer[:n])
		if err != nil {
			fmt.Printf("Error sending data: %v\n", err)
			return
		}

		totalSent += int64(n)

		// Progress indicator
		progress := float64(totalSent) / float64(fileSize) * 100
		fmt.Printf("\rProgress: %.2f%% (%d/%d bytes)", progress, totalSent, fileSize)
	}

	duration := time.Since(startTime)
	fmt.Printf("\nFile transfer completed in %v\n", duration)
	fmt.Printf("Average speed: %.2f KB/s\n", float64(totalSent)/1024/duration.Seconds())
	fmt.Println("Transfer successful!")
}
