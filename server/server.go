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

// SortedSet represents a sorted set with members and their scores
type SortedSet struct {
	members map[string]float64
}

// Member represents a member of the sorted set
type Member struct {
	Name  string
	Score float64
}

var db = make(map[string]string)
var ttl = make(map[string]time.Time)
var sortedSets = make(map[string]*SortedSet)

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
		case "KEYS":
			handleKeys(conn, cmd)
		case "ZADD":
			handleZAdd(conn, cmd)
		case "ZRANGE":
			handleZRange(conn, cmd)
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
		conn.Write([]byte(resp.EncodeBulkString("(nil)")))
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
func handleKeys(conn net.Conn, cmd []string) {
	if len(cmd) != 2 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'KEYS' command")))
		return
	}

	pattern := cmd[1]
	var matchingKeys []string

	// Lock the database for reading
	dbMutex.Lock()
	defer dbMutex.Unlock()

	// Iterate through simple key-value pairs in db
	for key := range db {
		if matchesPattern(key, pattern) {
			matchingKeys = append(matchingKeys, key)
		}
	}

	// Also check sortedSets for matching keys
	for key := range sortedSets {
		if matchesPattern(key, pattern) {
			matchingKeys = append(matchingKeys, key)
		}
	}

	// Encode the response as an array of matching keys
	conn.Write([]byte(resp.EncodeArray(matchingKeys)))
}

// Helper function to match a key against a pattern
func matchesPattern(key string, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(key, strings.TrimSuffix(pattern, "*"))
	}
	return key == pattern // Exact match
}

func handleZAdd(conn net.Conn, cmd []string) {
	if len(cmd) < 4 || (len(cmd)-2)%2 != 0 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'ZADD' command")))
		return
	}

	key := cmd[1]
	dbMutex.Lock()
	if sortedSets[key] == nil {
		sortedSets[key] = &SortedSet{members: make(map[string]float64)}
	}
	set := sortedSets[key]

	added := 0
	for i := 2; i < len(cmd); i += 2 {
		score, err := strconv.ParseFloat(cmd[i], 64)
		if err != nil {
			conn.Write([]byte(resp.EncodeError("ERR invalid score")))
			dbMutex.Unlock()
			return
		}
		member := cmd[i+1]
		if _, exists := set.members[member]; !exists {
			added++
		}
		set.members[member] = score
	}

	dbMutex.Unlock()
	conn.Write([]byte(resp.EncodeInteger(added)))
}

func handleZRange(conn net.Conn, cmd []string) {
	if len(cmd) != 4 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'ZRANGE' command")))
		return
	}

	key := cmd[1]
	start, err1 := strconv.Atoi(cmd[2])
	stop, err2 := strconv.Atoi(cmd[3])
	if err1 != nil || err2 != nil {
		conn.Write([]byte(resp.EncodeError("ERR invalid range arguments")))
		return
	}

	dbMutex.Lock()
	set, exists := sortedSets[key]
	dbMutex.Unlock()

	if !exists {
		conn.Write([]byte(resp.EncodeArray([]string{})))
		return
	}

	members := make([]Member, 0, len(set.members))
	for member, score := range set.members {
		members = append(members, Member{Name: member, Score: score})
	}

	// Sort members by score
	sort.Slice(members, func(i, j int) bool {
		return members[i].Score < members[j].Score
	})

	if start < 0 {
		start += len(members)
	}
	if stop < 0 {
		stop += len(members)
	}
	if stop >= len(members) {
		stop = len(members) - 1
	}

	if start > stop || start >= len(members) {
		conn.Write([]byte(resp.EncodeArray([]string{})))
		return
	}

	result := make([]string, stop-start+1)
	for i := start; i <= stop; i++ {
		result[i-start] = members[i].Name
	}

	conn.Write([]byte(resp.EncodeArray(result)))
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
