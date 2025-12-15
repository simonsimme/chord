# Chord

## flags allowed 
1. -a <String> = The IP address that the Chord client will bind to, (e.g., 128.8.126.63). Must be specified.
2. -p <Number> = The port that the Chord client will bind to and listen on. Must be specified.
3. -ja <String> = The IP address of the machine running a Chord node. The Chord client will join this node’s ring. Represented as an ASCII string (e.g., 128.8.126.63). Must be specified if --jp is specified.
4. -jp <Number> = The port that an existing Chord node is bound to and listening on. The Chord client will join this node’s ring. Must be specified if --ja is specified.

5. -ts <Number> = The time in milliseconds between invocations of ‘stabilize’. Represented as a base-10 integer. Must be specified, with a value in the range of [1,60000].
6. -tff <Number> = The time in milliseconds between invocations of ‘fix fingers’. Represented as a base-10 integer. Must be specified, with a value in the range of [1,60000].
7. -tcp <Number> = The time in milliseconds between invocations of ‘check predecessor’.
Represented as a base-10 integer. Must be specified, with a value in the range of [1,60000].
8. -r <Number> = The number of successors maintained by the Chord client. Represented as a base-10 integer. Must be specified, with a value in the range of [1,32].
9. -i <String> = The identifier (ID) assigned to the Chord client which will override the ID computed by the SHA1 sum of the client’s IP address and port number. Represented as a string of 40 characters matching [0-9a-fA-F]. Optional parameter.


## Compling 
This will build and compile the project into a file called chord. 
```bash
go build
```
## Creating the cert 
```bash
bash ./certs/generate_certs.sh 
```

## Starting the First Node
We are currently running on localhost:
```bash
./chord -a 127.0.0.1 -p 4170 --ts 3000 --tff 1000 --tcp 3000 -r 4
```

## Joining a Node (We need to change the port number if running on the same machine)
```bash
./chord -a 127.0.0.1 -p 4171 --ja 127.0.0.1 --jp 4170 --ts 3000 --tff 1000 --tcp 3000 -r 4
```
## How to Use the Chord Client

Available commands:
```
help              - Show this help message
ping <address>    - Ping another node (You can use :port for localhost)
Lookup <filename> <password>              - Lookup the node responsible for a key
StoreFile <local path/filename> <password> - Store a file in the DHT
dump              - Display info about the current node
quit              - Exit the program
```