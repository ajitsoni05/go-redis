# go-redis

A simple in-memory Redis-like server built in Go.

## Running the Server

To run the server, execute:
```bash
go run server.go
```
To use the client, execute:

```bash
go run client.go
```

## RESP BASIC DESCRIPTION


REDIS Assignment



-> SET
https://redis.io/docs/latest/commands/set/

USAGE -
SET key value [NX | XX] [GET] [EX seconds | PX milliseconds | EXAT unix-time-seconds | PXAT unix-time-milliseconds | KEEPTTL]

.. Set key to hold the string value.
.. If key already holds a value, it is overwritten, regardless of its type.
.. Any previous time to live associated with the key is discarded on successful SET operation.


-> GET
https://redis.io/docs/latest/commands/get/

USAGE -
GET key

.. Get the value of key.
.. If the key does not exist the special value nil is returned.
.. An error is returned if the value stored at key is not string, GET only handles string values.

-> DEL
https://redis.io/docs/latest/commands/del/

USAGE -
DEL key [key ...]
.. O(N) where N is the number of keys that will be removed
.. When a key to remove holds a value other than a string | the individual complexity for this key is O(M) where M is the number of elements in the list, set, sorted set or hash.
.. Removing a single key that holds a string value is O(1).

-> EXPIRE
https://redis.io/docs/latest/commands/expire/

USAGE -
EXPIRE key seconds [NX | XX | GT | LT]

NX -- Set expiry only when the key has no expiry
XX -- Set expiry only when the key has an existing expiry
GT -- Set expiry only when the new expiry is greater than current one
LT -- Set expiry only when the new expiry is less than current on

.. Set a timeout on key.
.. After the timeout has expired, the key will automatically be deleted.
.. A key with an associated timeout is often said to be volatile in Redis terminology.

-> KEYS
https://redis.io/docs/latest/commands/keys/
While the time complexity for this operation is O(N), the constant times are fairly low


-> TTL
https://redis.io/docs/latest/commands/ttl/


-> ZADD
https://redis.io/docs/latest/commands/zadd/
ZADD key [NX | XX] [GT | LT] [CH] [INCR] score member [score member ...]
... O(log(N)) for each item added, where N is the number of elements in the sorted set.
...

-> ZRANGE
https://redis.io/docs/latest/commands/zrange/






