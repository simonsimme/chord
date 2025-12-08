package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	pb "chord/protocol" // Update path as needed

	"google.golang.org/grpc"
)

var localaddress string

// Find our local IP address
func init() {
	// Configure log package to show short filename, line number and timestamp with only time
	log.SetFlags(log.Lshortfile | log.Ltime)

	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	localaddress = localAddr.IP.String()

	if localaddress == "" {
		panic("init: failed to find non-loopback interface with valid address on this node")
	}
	log.Printf("found local address %s\n", localaddress)
}

// resolveAddress handles :port format by adding the local address
func resolveAddress(address string) string {
	if strings.HasPrefix(address, ":") {
		return net.JoinHostPort(localaddress, address[1:])
	} else if !strings.Contains(address, ":") {
		return net.JoinHostPort(address, defaultPort)
	}
	return address
}

// StartServer starts the gRPC server for this node
func StartServer(address string, nprime string, ts int, tff int, tcp int) (*Node, error) {
	address = resolveAddress(address)

	node := &Node{
		Address:     address,
		FingerTable: make([]string, keySize+1),
		Predecessor: "",
		Successors:  nil,
		Bucket:      make(map[string]string),
	}

	// Are we the first node?
	if nprime == "" {
		log.Print("StartServer: creating new ring")
		node.Successors = []string{node.Address}
	} else {
		log.Print("StartServer: joining existing ring using ", nprime)
		// For now use the given address as our successor
		nprime = resolveAddress(nprime)
		node.Successors = []string{nprime}
		// TODO: use a GetAll request to populate our bucket

	}

	// Start listening for RPC calls
	grpcServer := grpc.NewServer()
	pb.RegisterChordServer(grpcServer, node)

	lis, err := net.Listen("tcp", node.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	// Start server in goroutine
	log.Printf("Starting Chord node server on %s", node.Address)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// Start background tasks
	go func() {
		wait := time.Second / 3
		if ts == 0 {
			wait = time.Duration(ts) * time.Millisecond
		}
		for {
			time.Sleep(wait)
			node.stabilize()
		}
	}()
	go func() {
		wait := time.Second / 3
		if ts == 0 {
			wait = time.Duration(tff) * time.Millisecond
		}
		nextFinger := 0
		for {
			time.Sleep(wait)
			nextFinger = node.fixFingers(nextFinger)
		}
	}()
	go func() {
		wait := time.Second / 3
		if ts == 0 {
			wait = time.Duration(tcp) * time.Millisecond
		}
		for {
			time.Sleep(wait)
			node.checkPredecessor()
		}
	}()

	return node, nil
}

// RunShell provides an interactive command shell
func RunShell(node *Node) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nExiting...")
				return
			}
			fmt.Println("Error reading input:", err)
			continue
		}

		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		switch parts[0] {
		case "help":
			fmt.Println("Available commands:")
			fmt.Println("  help              - Show this help message")
			fmt.Println("  ping <address>    - Ping another node")
			fmt.Println("                      (You can use :port for localhost)")
			fmt.Println("  put <key> <value> <address> - Store a key-value pair on a node")
			fmt.Println("  get <key> <address>         - Get a value for a key from a node")
			fmt.Println("  delete <key> <address>      - Delete a key from a node")
			fmt.Println("  getall <address>            - Get all key-value pairs from a node")
			fmt.Println("  dump              - Display info about the current node")
			fmt.Println("  quit              - Exit the program")

		case "ping":
			if len(parts) < 2 {
				fmt.Println("Usage: ping <address>")
				continue
			}

			err := PingNode(ctx, parts[1])
			if err != nil {
				fmt.Printf("Ping failed: %v\n", err)
			} else {
				fmt.Println("Ping successful")
			}

		case "put":
			if len(parts) < 4 {
				fmt.Println("Usage: put <key> <value> <address>")
				continue
			}

			err := PutKeyValue(ctx, parts[1], parts[2], parts[3])
			if err != nil {
				fmt.Printf("Put failed: %v\n", err)
			} else {
				fmt.Printf("Put successful: %s -> %s\n", parts[1], parts[2])
			}

		case "get":
			if len(parts) < 3 {
				fmt.Println("Usage: get <key> <address>")
				continue
			}

			value, err := GetValue(ctx, parts[1], parts[2])
			if err != nil {
				fmt.Printf("Get failed: %v\n", err)
			} else if value == "" {
				fmt.Printf("Key '%s' not found\n", parts[1])
			} else {
				fmt.Printf("%s -> %s\n", parts[1], value)
			}

		case "delete":
			if len(parts) < 3 {
				fmt.Println("Usage: delete <key> <address>")
				continue
			}

			err := DeleteKey(ctx, parts[1], parts[2])
			if err != nil {
				fmt.Printf("Delete failed: %v\n", err)
			} else {
				fmt.Printf("Delete request for key '%s' completed\n", parts[1])
			}

		case "getall":
			if len(parts) < 2 {
				fmt.Println("Usage: getall <address>")
				continue
			}

			keyValues, err := GetAllKeyValues(ctx, parts[1])
			if err != nil {
				fmt.Printf("GetAll failed: %v\n", err)
			} else {
				if len(keyValues) == 0 {
					fmt.Println("No key-value pairs found")
				} else {
					fmt.Println("Key-value pairs:")
					for k, v := range keyValues {
						fmt.Printf("  %s -> %s\n", k, v)
					}
				}
			}

		case "dump":
			node.dump()

		case "quit":
			fmt.Println("Exiting...")
			return

		default:
			fmt.Println("Unknown command. Type 'help' for available commands.")
		}
	}
}

func main() {
	// Parse command line flags
	//createCmd := flag.NewFlagSet("create", flag.ExitOnError)
	//createPort := createCmd.Int("port", 3410, "Port to listen on")

	//joinCmd := flag.NewFlagSet("join", flag.ExitOnError)
	//joinPort := joinCmd.Int("port", 3410, "Port to listen on")
	//joinAddr := joinCmd.String("addr", "", "Address of existing node")

	if len(os.Args) < 2 {
		fmt.Println("Expected at least -a and -p arguments")
		os.Exit(1)
	}

	var node *Node
	var address string
	var port string
	var ts int
	var tff int
	var tcpT int
	var ja string
	var r int
	var jp int
	var identifier string
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-a":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for -a")
			}
			address = os.Args[i+1]
			i++
		case "-p":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for -p")
			}
			port = os.Args[i+1]
			i++
		case "--ja":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for --ja")
			}
			ja = os.Args[i+1]
			i++
		case "--jp":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for --jp")
			}
			v, err := strconv.Atoi(os.Args[i+1])
			if err != nil {
				log.Fatalf("invalid integer for --jp: %v", err)
			}
			jp = v
			i++
		case "--ts":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for --ts")
			}
			v, err := strconv.Atoi(os.Args[i+1])
			if err != nil {
				log.Fatalf("invalid integer for --ts: %v", err)
			}
			if !(v <= 6000 && v >= 1) {
				log.Fatal("--ts must be between 1 and 6000")
			}
			ts = v
			i++
		case "--tff":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for --tff")
			}
			v, err := strconv.Atoi(os.Args[i+1])
			if err != nil {
				log.Fatalf("invalid integer for --tff: %v", err)
			}
			if !(v <= 6000 && v >= 1) {
				log.Fatal("--tff must be between 1 and 6000")
			}
			tff = v
			i++
		case "--tcp":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for --tcp")
			}
			v, err := strconv.Atoi(os.Args[i+1])
			if err != nil {
				log.Fatalf("invalid integer for --tcp: %v", err)
			}
			if !(v <= 6000 && v >= 1) {
				log.Fatal("--tcp must be between 1 and 6000")
			}
			tcpT = v
			i++
		case "-r":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for -r")
			}
			v, err := strconv.Atoi(os.Args[i+1])
			if err != nil {
				log.Fatalf("invalid integer for -r: %v", err)
			}
			if !(v <= 32 && v >= 1) {
				log.Fatal("-r must be between 1 and 32")
			}
			r = v
			i++
		case "-i":
			if i+1 >= len(os.Args) {
				log.Fatal("missing value for -i")
			}
			str := os.Args[i+1]
			if len(str) == 40 {
				log.Fatal("-i identifier must be 40 characters")
			}
			identifier = str
			i++
		default:
			log.Fatalf("unknown argument: %s", os.Args[i])
		}
	}

	log.Printf("Parsed arguments: address=%s, port=%s, ja=%s, jp=%d, ts=%d, tff=%d, tcp=%d, r=%d, id=%s\n", address, port, ja, jp, ts, tff, tcpT, r, identifier)
	if address == "" || port == "" {
		log.Fatal("address and port must be specified with -a and -p")
	}
	if ja == "" && jp == 0 {
		//Create
		node, err := StartServer(address+":"+port, "", ts, tff, tcpT)
		if err != nil {
			log.Fatalf("Failed to create ring: %v", err)
		}
		log.Printf("Created new ring with node at %s", node.Address)

	} else if ja != "" {
		if jp == 0 {
			log.Fatal("--jp must be specified when --ja is used")
		}
		//Join
		node, err := StartServer(address+":"+port, ja+":"+strconv.Itoa(jp), ts, tff, tcpT)
		if err != nil {
			log.Fatalf("Failed to join ring: %v", err)
		}
		log.Printf("Joined ring with node at %s", node.Address)

	}

	// Run the interactive shell
	RunShell(node)
}
