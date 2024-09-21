package main

import (
	"bufio"
	"fmt"
	"go-redis/resp"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// In-memory database
var db = make(map[string]string)
var ttl = make(map[string]time.Time)

// Mutex for concurrent access to the database
var dbMutex sync.Mutex

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		// Decode client request (RESP array)
		cmd, err := resp.DecodeArray(reader)
		if err != nil {
			conn.Write([]byte(resp.EncodeError("ERR invalid request")))
			return
		}

		// Log the received command
		fmt.Println("Received command:", cmd)

		// Handle the command
		if len(cmd) == 0 {
			conn.Write([]byte(resp.EncodeError("ERR no command")))
			continue
		}

		command := strings.ToUpper(cmd[0])
		switch command {
		case "GET":
			handleGet(conn, cmd)
		case "SET":
			handleSet(conn, cmd)
		case "DEL":
			handleDel(conn, cmd)
		case "EXPIRE":
			handleExpire(conn, cmd)
		case "TTL":
			handleTTL(conn, cmd)
		default:
			conn.Write([]byte(resp.EncodeError("ERR unknown command")))
		}
	}
}

func handleGet(conn net.Conn, cmd []string) {
	if len(cmd) != 2 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'GET' command")))
		return
	}

	key := cmd[1]

	// Handling Concurrency
	dbMutex.Lock()
	value, ok := db[key]
	dbMutex.Unlock()

	if ok {
		if expiration, exists := ttl[key]; exists && time.Now().After(expiration) {
			delete(db, key)
			delete(ttl, key)
			conn.Write([]byte(resp.EncodeBulkString("")))
		} else {
			conn.Write([]byte(resp.EncodeBulkString(value)))
		}
	} else {
		conn.Write([]byte(resp.EncodeBulkString("")))
	}
}

func handleSet(conn net.Conn, cmd []string) {
	if len(cmd) != 3 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'SET' command")))
		return
	}

	fmt.Println("SET", cmd)
	key, value := cmd[1], cmd[2]

	// Handling Concurrency
	dbMutex.Lock()
	db[key] = value
	dbMutex.Unlock()

	conn.Write([]byte(resp.EncodeSimpleString("OK")))
}

func handleDel(conn net.Conn, cmd []string) {
	if len(cmd) < 2 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'DEL' command")))
		return
	}

	deleted := 0
	for _, key := range cmd[1:] {
		if _, ok := db[key]; ok {
			delete(db, key)
			delete(ttl, key)
			deleted++
		}
	}
	conn.Write([]byte(resp.EncodeInteger(deleted)))
}

func handleExpire(conn net.Conn, cmd []string) {
	if len(cmd) != 3 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'EXPIRE' command")))
		return
	}

	key := cmd[1]
	expirySeconds, err := strconv.Atoi(cmd[2])
	if err != nil {
		conn.Write([]byte(resp.EncodeError("ERR invalid expiration time")))
		return
	}

	dbMutex.Lock()
	_, exists := db[key]
	dbMutex.Unlock()

	if exists {
		ttl[key] = time.Now().Add(time.Duration(expirySeconds) * time.Second)
		conn.Write([]byte(resp.EncodeInteger(1)))
	} else {
		conn.Write([]byte(resp.EncodeInteger(0)))
	}
}

func handleTTL(conn net.Conn, cmd []string) {
	if len(cmd) != 2 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'TTL' command")))
		return
	}

	key := cmd[1]
	if expiration, exists := ttl[key]; exists {
		if time.Now().After(expiration) {
			conn.Write([]byte(resp.EncodeInteger(-2)))
		} else {
			ttlRemaining := int(expiration.Sub(time.Now()).Seconds())
			conn.Write([]byte(resp.EncodeInteger(ttlRemaining)))
		}
	} else if _, exists := db[key]; exists {
		conn.Write([]byte(resp.EncodeInteger(-1)))
	} else {
		conn.Write([]byte(resp.EncodeInteger(-2)))
	}
}

func main() {
	ln, err := net.Listen("tcp", ":6378")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer ln.Close()

	fmt.Println("Server started on :6378")

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}
		go handleConnection(conn)
	}
}
