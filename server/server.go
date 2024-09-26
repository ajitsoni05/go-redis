package main

import (
	"bufio"
	"fmt"
	"go-redis/resp"
	"net"
	"sort"
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
		// Decode client request (RESP array) int array of commands
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
		// Based on the first word in the cmd array go to the handler
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
	// get command should consist of two elements | Make a check here
	if len(cmd) != 2 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'GET' command")))
		return
	}

	// 0th element in command is GET and 1st element is the key to which value is to be found out
	key := cmd[1]

	// Handling Concurrency
	dbMutex.Lock()
	value, ok := db[key]
	dbMutex.Unlock()

	if ok {
		// Check if the expiration exists for the key and whether expiration time has exceeded current time
		if expiration, exists := ttl[key]; exists && time.Now().After(expiration) {
			// delete the specified key from db datasore
			delete(db, key)
			// delete the expiration for the key from TTL datastore
			delete(ttl, key)
			conn.Write([]byte(resp.EncodeBulkString("(nil)")))
		} else {
			conn.Write([]byte(resp.EncodeBulkString(value)))
		}
	} else {
		conn.Write([]byte(resp.EncodeBulkString("(nil)")))
	}
}

func handleSet(conn net.Conn, cmd []string) {
	if len(cmd) < 3 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'SET' command")))
		return
	}

	key, value := cmd[1], cmd[2]
	var (
		nx       = false
		xx       = false
		get      = false
		expiry   time.Duration
		expireAt time.Time
		keepttl  = false
	)

	// Parsing options
	i := 3
	for i < len(cmd) {
		switch strings.ToUpper(cmd[i]) {
		case "NX":
			nx = true
		case "XX":
			xx = true
		case "GET":
			get = true
		case "EX":
			if i+1 >= len(cmd) {
				conn.Write([]byte(resp.EncodeError("ERR syntax error")))
				return
			}
			seconds, err := strconv.Atoi(cmd[i+1])
			if err != nil || seconds <= 0 {
				conn.Write([]byte(resp.EncodeError("ERR invalid expire time in SET")))
				return
			}
			expiry = time.Duration(seconds) * time.Second
			i++
		case "PX":
			if i+1 >= len(cmd) {
				conn.Write([]byte(resp.EncodeError("ERR syntax error")))
				return
			}
			milliseconds, err := strconv.Atoi(cmd[i+1])
			if err != nil || milliseconds <= 0 {
				conn.Write([]byte(resp.EncodeError("ERR invalid expire time in SET")))
				return
			}
			expiry = time.Duration(milliseconds) * time.Millisecond
			i++
		case "EXAT":
			if i+1 >= len(cmd) {
				conn.Write([]byte(resp.EncodeError("ERR syntax error")))
				return
			}
			unixTime, err := strconv.ParseInt(cmd[i+1], 10, 64)
			if err != nil || unixTime <= 0 {
				conn.Write([]byte(resp.EncodeError("ERR invalid Unix time")))
				return
			}
			expireAt = time.Unix(unixTime, 0)
			i++
		case "PXAT":
			if i+1 >= len(cmd) {
				conn.Write([]byte(resp.EncodeError("ERR syntax error")))
				return
			}
			unixTimeMs, err := strconv.ParseInt(cmd[i+1], 10, 64)
			if err != nil || unixTimeMs <= 0 {
				conn.Write([]byte(resp.EncodeError("ERR invalid Unix time")))
				return
			}
			expireAt = time.Unix(0, unixTimeMs*int64(time.Millisecond))
			i++
		case "KEEPTTL":
			keepttl = true
		default:
			conn.Write([]byte(resp.EncodeError("ERR syntax error")))
			return
		}
		i++
	}
	// Handling concurrency for database write
	dbMutex.Lock()
	defer dbMutex.Unlock()

	oldValue, exists := db[key]
	if nx && exists {
		// NX: Only set if the key does not exist
		conn.Write([]byte(resp.EncodeBulkString("")))
		return
	}
	if xx && !exists {
		// XX: Only set if the key already exists
		conn.Write([]byte(resp.EncodeBulkString("(nil)")))
		return
	}

	// Handle the GET option: Return the old value before overwriting
	if get {
		if exists {
			conn.Write([]byte(resp.EncodeBulkString(oldValue)))
		} else {
			conn.Write([]byte(resp.EncodeBulkString("(nil)")))
		}
	}

	// Set the new value
	db[key] = value

	// Handle the expiry options
	if keepttl && exists {
		// KEEPTTL: Retain the existing TTL, do nothing
	} else if !expireAt.IsZero() {
		ttl[key] = expireAt
	} else if expiry > 0 {
		ttl[key] = time.Now().Add(expiry)
	} else {
		delete(ttl, key) // Remove TTL if no expiration is specified
	}

	conn.Write([]byte(resp.EncodeSimpleString("OK")))
}

func handleDel(conn net.Conn, cmd []string) {
	if len(cmd) < 2 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'DEL' command")))
		return
	}

	deleted := 0
	/*
		DEL keyword can be used for deleting multiple keys at once
		so in below logic we are deleting the db entries and the ttl entries
		associated with the corresponding keys
	*/
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
	if len(cmd) < 3 || len(cmd) > 4 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'EXPIRE' command")))
		return
	}

	key := cmd[1]
	expirySeconds, err := strconv.Atoi(cmd[2])
	if err != nil {
		conn.Write([]byte(resp.EncodeError("ERR invalid expiration time")))
		return
	}

	var option string
	if len(cmd) == 4 {
		option = strings.ToUpper(cmd[3])
		if option != "NX" && option != "XX" && option != "GT" && option != "LT" {
			conn.Write([]byte(resp.EncodeError("ERR invalid option for 'EXPIRE' command")))
			return
		}
	}

	dbMutex.Lock()
	existingTTL, hasTTL := ttl[key]
	_, exists := db[key]
	dbMutex.Unlock()

	// Handle NX (Only set expiration if the key does not already exist)
	if option == "NX" && hasTTL {
		conn.Write([]byte(resp.EncodeInteger(0)))
		return
	}

	// Handle XX (Only set expiration if the key already exists)
	if option == "XX" && !hasTTL {
		conn.Write([]byte(resp.EncodeInteger(0)))
		return
	}

	// Handle GT (Only set expiration if the new expiry time is greater than the current one)
	if option == "GT" && hasTTL && existingTTL.After(time.Now()) && existingTTL.Sub(time.Now()).Seconds() > float64(expirySeconds) {
		conn.Write([]byte(resp.EncodeInteger(0)))
		return
	}

	// Handle LT (Only set expiration if the new expiry time is less than the current one)
	if option == "LT" && hasTTL && existingTTL.After(time.Now()) && existingTTL.Sub(time.Now()).Seconds() < float64(expirySeconds) {
		conn.Write([]byte(resp.EncodeInteger(0)))
		return
	}

	// If the key exists, set the new expiration time
	if exists {
		dbMutex.Lock()
		ttl[key] = time.Now().Add(time.Duration(expirySeconds) * time.Second)
		dbMutex.Unlock()
		conn.Write([]byte(resp.EncodeInteger(1)))
	} else {
		conn.Write([]byte(resp.EncodeInteger(0)))
	}
}

func handleTTL(conn net.Conn, cmd []string) {
	// Command length check | Should be equal to 2
	/*
		Example Usage :
			 TTL keyname
		Output:
		(integer) 2000

	*/
	if len(cmd) != 2 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'TTL' command")))
		return
	}

	key := cmd[1]
	if expiration, exists := ttl[key]; exists {
		if time.Now().After(expiration) {
			conn.Write([]byte(resp.EncodeInteger(-2)))
		} else {
			// Simple subtraction logic to indicate time remaining
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

	/*
		 Following check on commands are based on two conditions:
		1. length of the command should not be less than 4 becuz, we need to add atleast 1 member with its scode
		ex :  zadd mysortedset 1 "a"
		2. after the first two members of the command i.e. zadd , mysortedset . The number of following arguemtns key ,scode is
		always even so we check that.

	*/
	if len(cmd) < 4 || (len(cmd)-2)%2 != 0 {
		conn.Write([]byte(resp.EncodeError("ERR wrong number of arguments for 'ZADD' command")))
		return
	}

	key := cmd[1]
	// Lock the  db datastore for manipulation of key in sortedSets
	dbMutex.Lock()
	// add the key into sorted set if not already present
	if sortedSets[key] == nil {
		sortedSets[key] = &SortedSet{members: make(map[string]float64)}
	}
	// else get the corresponding  sortedset
	set := sortedSets[key]

	added := 0
	// get the keys and scores to be added/updated in the data store
	for i := 2; i < len(cmd); i += 2 {
		// score can be a floating value as well
		score, err := strconv.ParseFloat(cmd[i], 64)
		if err != nil {
			conn.Write([]byte(resp.EncodeError("ERR invalid score")))
			dbMutex.Unlock()
			return
		}
		// member would be next index to score
		member := cmd[i+1]
		// add the member to the corresponding set members
		if _, exists := set.members[member]; !exists {
			added++
		}
		// update the score
		set.members[member] = score
	}

	// Unlok after the keys are released
	dbMutex.Unlock()
	conn.Write([]byte(resp.EncodeInteger(added)))
}

func handleZRange(conn net.Conn, cmd []string) {
	// Check on the length of the command | if its not equal to 4 , then its invalid
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
	// listener closure at the end
	defer ln.Close()

	// Print statement to indicate that server is started on port 8020
	fmt.Println("Server started on :8020")

	// While loop for handling the connection
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}
		// asynchronous connection handling
		go handleConnection(conn)
	}
}
