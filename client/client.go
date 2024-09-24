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
	case '+': // Simple String
		return strings.TrimSpace(line[1:]), nil
	case '-': // Error
		return strings.TrimSpace(line), nil
	case ':': // Integer (handle "(integer) x" format)
		value, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(integer) %d", value), nil
	case '$': // Bulk String
		lengthStr := strings.TrimSpace(line[1:])
		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			return "", err
		}
		if length == -1 {
			return "(nil)", nil
		}

		data := make([]byte, length+2) // Read the bulk string and the trailing \r\n
		_, err = reader.Read(data)
		if err != nil {
			return "", err
		}
		return string(data[:length]), nil
	case '*': // Array
		length, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return "", err
		}
		if length == -1 {
			return "(nil)", nil
		}

		array := make([]string, length)
		for i := 0; i < length; i++ {
			elem, err := readRespResponse(reader)
			if err != nil {
				return "", err
			}
			array[i] = elem
		}
		return fmt.Sprintf("%v", array), nil
	default:
		return "", fmt.Errorf("unexpected response type: %s", line)
	}
}
