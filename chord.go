package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log"
	"math/big"
	"sync"

	pb "chord/protocol" // Update path as needed

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultPort       = "3410"
	successorListSize = 3
	keySize           = sha1.Size * 8
	maxLookupSteps    = 32
)

var (
	two     = big.NewInt(2)
	hashMod = new(big.Int).Exp(big.NewInt(2), big.NewInt(keySize), nil)
)

// Node represents a node in the Chord DHT
type Node struct {
	pb.UnimplementedChordServer
	mu sync.RWMutex

	Address     string
	Predecessor string
	Successors  []string
	FingerTable []string

	Bucket map[string]string
}

// get the sha1 hash of a string as a bigint
func hash(elt string) *big.Int {
	hasher := sha1.New()
	hasher.Write([]byte(elt))
	return new(big.Int).SetBytes(hasher.Sum(nil))
}

// calculate the address of a point somewhere across the ring
// this gets the target point for a given finger table entry
// the successor of this point is the finger table entry
func jump(address string, fingerentry int) *big.Int {
	n := hash(address)

	fingerentryminus1 := big.NewInt(int64(fingerentry) - 1)
	distance := new(big.Int).Exp(two, fingerentryminus1, nil)

	sum := new(big.Int).Add(n, distance)

	return new(big.Int).Mod(sum, hashMod)
}

// returns true if elt is between start and end, accounting for the right
// if inclusive is true, it can match the end
func between(start, elt, end *big.Int, inclusive bool) bool {
	if end.Cmp(start) > 0 {
		return (start.Cmp(elt) < 0 && elt.Cmp(end) < 0) || (inclusive && elt.Cmp(end) == 0)
	} else {
		return start.Cmp(elt) < 0 || elt.Cmp(end) < 0 || (inclusive && elt.Cmp(end) == 0)
	}
}

// Ping implements the Ping RPC method
func (n *Node) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	log.Print("ping: received request")
	return &pb.PingResponse{}, nil
}

// Put implements the Put RPC method
func (n *Node) Put(ctx context.Context, req *pb.PutRequest) (*pb.PutResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	log.Print("put: [", req.Key, "] => [", req.Value, "]")
	n.Bucket[req.Key] = req.Value
	return &pb.PutResponse{}, nil
}

// Get implements the Get RPC method
func (n *Node) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	value, exists := n.Bucket[req.Key]
	if !exists {
		log.Print("get: [", req.Key, "] miss")
		return &pb.GetResponse{Value: ""}, nil
	}
	log.Print("get: [", req.Key, "] found [", value, "]")
	return &pb.GetResponse{Value: value}, nil
}

// Delete implements the Delete RPC method
func (n *Node) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, exists := n.Bucket[req.Key]; exists {
		log.Print("delete: found and deleted [", req.Key, "]")
		delete(n.Bucket, req.Key)
	} else {
		log.Print("delete: not found [", req.Key, "]")
	}
	return &pb.DeleteResponse{}, nil
}

// GetAll implements the GetAll RPC method
func (n *Node) GetAll(ctx context.Context, req *pb.GetAllRequest) (*pb.GetAllResponse, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	log.Printf("getall: returning %d key-value pairs", len(n.Bucket))

	// Create a copy of the bucket map
	keyValues := make(map[string]string)
	for k, v := range n.Bucket {
		keyValues[k] = v
	}

	return &pb.GetAllResponse{KeyValues: keyValues}, nil
}

func (n *Node) checkPredecessor() {
	// TODO: Student will implement this
}

func (n *Node) stabilize() {
	// TODO: Student will implement this
	log.Printf("stabilize: checking successor %s", n.Successors[0])
	n.mu.RLock()
	succ := n.Successors[0]
	n.mu.RUnlock()
	if n.Address == succ {
		return
	}
	succ = resolveAddress(succ)
	for i := 0; i < 5; i++ {
		// ask successor for its predecessor
		var resp pb.GetPredecessorResponse
		err := call(succ, "GetPredecessor", &pb.GetPredecessorRequest{}, &resp)
		if err != nil {
			log.Printf("stabilize: GetPredecessor call failed: %v", err)

		}
		if resp.Predecessor == "" {
			log.Printf("stabilize: got empty predecessor from ", succ)
			var resp pb.NotifyResponse
			err := call(succ, "Notify", &pb.NotifyRequest{Address: n.Address}, &resp)
			if err != nil {
				log.Printf("stabilize: notify call failed: %v", err)
			}
			break
		}
		if resp.Predecessor == n.Address {
			n.mu.Lock()
			n.Successors[0] = succ
			n.mu.Unlock()
			log.Printf("stabilize: successor is %s", succ)
			break
		} else {
			succ = resp.Predecessor
		}
	}
	log.Printf("stab end")

}
func (n *Node) Notify(ctx context.Context, req *pb.NotifyRequest) (*pb.NotifyResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	log.Printf("notify: received notification from %s", req.Address)
	n.Predecessor = req.Address
	return &pb.NotifyResponse{}, nil
}

func call(address string, method string, request interface{}, reply interface{}) error {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	client := pb.NewChordClient(conn)
	log.Printf("call: calling %s on %s", method, address)
	switch method {
	case "GetPredecessor":
		req, ok := request.(*pb.GetPredecessorRequest)
		if !ok {
			return fmt.Errorf("invalid request type for GetPredecessor")
		}
		resp, err := client.GetPredecessor(context.Background(), req)
		if err != nil {
			return err
		}
		r, ok := reply.(*pb.GetPredecessorResponse)
		if !ok {
			return fmt.Errorf("invalid reply type for GetPredecessor")
		}
		*r = *resp
	case "Notify":
		log.Printf("call: in Notify case")
		req, ok := request.(*pb.NotifyRequest)
		if !ok {
			return fmt.Errorf("invalid request type for Notify")
		}
		resp, err := client.Notify(context.Background(), req)
		if err != nil {
			return err
		}
		r, ok := reply.(*pb.NotifyResponse)
		if !ok {
			return fmt.Errorf("invalid reply type for Notify")
		}
		*r = *resp
	default:
		return fmt.Errorf("unknown method: %s", method)
	}
	log.Printf("call: completed %s on %s", method, address)
	return nil
}

// GetPredecessor implements the GetPredecessor RPC method
func (n *Node) GetPredecessor(ctx context.Context, req *pb.GetPredecessorRequest) (*pb.GetPredecessorResponse, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return &pb.GetPredecessorResponse{Predecessor: n.Predecessor}, nil
}

func (n *Node) fixFingers(nextFinger int) int {
	// TODO: Student will implement this
	nextFinger++
	if nextFinger > keySize {
		nextFinger = 1
	}
	return nextFinger
}

// format an address for printing
func addr(a string) string {
	if a == "" {
		return "(empty)"
	}
	s := fmt.Sprintf("%040x", hash(a))
	return s[:8] + ".. (" + a + ")"
}

// print useful info about the local node
func (n *Node) dump() {
	n.mu.RLock()
	defer n.mu.RUnlock()

	fmt.Println()
	fmt.Println("Dump: information about this node")

	// predecessor and successor links
	fmt.Println("Neighborhood")
	fmt.Println("pred:   ", addr(n.Predecessor))
	fmt.Println("self:   ", addr(n.Address))
	for i, succ := range n.Successors {
		fmt.Printf("succ  %d: %s\n", i, addr(succ))
	}
	fmt.Println()
	fmt.Println("Finger table")
	i := 1
	for i <= keySize {
		for i < keySize && n.FingerTable[i] == n.FingerTable[i+1] {
			i++
		}
		fmt.Printf(" [%3d]: %s\n", i, addr(n.FingerTable[i]))
		i++
	}
	fmt.Println()
	fmt.Println("Data items")
	for k, v := range n.Bucket {
		s := fmt.Sprintf("%040x", hash(k))
		fmt.Printf("    %s.. %s => %s\n", s[:8], k, v)
	}
	fmt.Println()
}
