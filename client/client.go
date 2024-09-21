package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:6378")
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Enter command: ")
		command, _ := reader.ReadString('\n')
		command = strings.TrimSpace(command)

		_, err := conn.Write([]byte(encodeArray(strings.Split(command, " "))))
		if err != nil {
			fmt.Println("Error sending command:", err)
			return
		}

		responseReader := bufio.NewReader(conn)
		response, err := readRespResponse(responseReader)
		if err != nil {
			fmt.Println("Error reading response:", err)
			return
		}

		fmt.Println("Response:", response)
	}
}

func encodeArray(values []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*%d\r\n", len(values)))
	for _, val := range values {
		sb.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val))
	}
	return sb.String()
}

func readRespResponse(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	switch line[0] {
	case '+', ':', '-':
		return strings.TrimSpace(line), nil
	case '$':
		lengthStr := strings.TrimSpace(line[1:])
		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			return "", err
		}

		if length == -1 {
			return "(nil)", nil
		}

		data := make([]byte, length+2)
		_, err = reader.Read(data)
		if err != nil {
			return "", err
		}
		return string(data[:length]), nil
	default:
		return "", fmt.Errorf("unexpected response type: %s", line)
	}
}
