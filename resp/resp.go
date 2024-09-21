package resp

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

func EncodeSimpleString(value string) string {
	return fmt.Sprintf("+%s\r\n", value)
}

func EncodeBulkString(value string) string {
	return fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
}

func EncodeInteger(value int) string {
	return fmt.Sprintf(":%d\r\n", value)
}

func EncodeError(message string) string {
	return fmt.Sprintf("-%s\r\n", message)
}

func EncodeArray(values []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*%d\r\n", len(values)))
	for _, val := range values {
		sb.WriteString(EncodeBulkString(val))
	}
	return sb.String()
}

func DecodeArray(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	if line[0] != '*' {
		return nil, fmt.Errorf("Expected array, got %s", line)
	}

	length, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}

	result := make([]string, length)

	for i := 0; i < length; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		if line[0] != '$' {
			return nil, fmt.Errorf("Expected bulk string, got %s", line)
		}

		strLen, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return nil, err
		}

		buf := make([]byte, strLen+2) // +2 for \r\n
		_, err = reader.Read(buf)
		if err != nil {
			return nil, err
		}

		result[i] = string(buf[:strLen])
	}

	return result, nil
}
