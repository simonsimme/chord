package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"
	"sync"

	pb "chord/protocol" // Update path as needed

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	defaultPort = "3410"
	//successorListSize = 20
	keySize        = sha1.Size * 8
	maxLookupSteps = 32
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

	Bucket map[string][]byte

	SuccessorListSize int
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
	//log.Print("ping: received request")
	return &pb.PingResponse{}, nil
}

// Put implements the Put RPC method
func (n *Node) Put(ctx context.Context, req *pb.PutRequest) (*pb.PutResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	//log.Print("put: [", req.Key, "] => [", req.Value, "]")
	n.Bucket[req.Key] = req.Value
	return &pb.PutResponse{}, nil
}

// Get implements the Get RPC method
func (n *Node) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	value, exists := n.Bucket[req.Key]
	if !exists {
		//log.Print("get: [", req.Key, "] miss")
		return &pb.GetResponse{Value: nil}, nil
	}
	//log.Print("get: [", req.Key, "] found [", value, "]")
	return &pb.GetResponse{Value: value}, nil
}

// Delete implements the Delete RPC method
func (n *Node) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, exists := n.Bucket[req.Key]; exists {
		//log.Print("delete: found and deleted [", req.Key, "]")
		delete(n.Bucket, req.Key)
	} else {
		//log.Print("delete: not found [", req.Key, "]")
	}
	return &pb.DeleteResponse{}, nil
}

// GetAll implements the GetAll RPC method
func (n *Node) GetAll(ctx context.Context, req *pb.GetAllRequest) (*pb.GetAllResponse, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	//log.Printf("getall: returning %d key-value pairs", len(n.Bucket))

	// Create a copy of the bucket map
	keyValues := make(map[string][]byte)
	for k, v := range n.Bucket {
		keyValues[k] = v
	}

	return &pb.GetAllResponse{KeyValues: keyValues}, nil
}
func (n *Node) StoreFile(filepath string) error {
	fileData, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}
	filename := path.Base(filepath)

	//fileID := hash(filename)
	//out file on key responsible
	_, targetAddress, _, err := n.Lookup(filename)
	if err != nil {
		return fmt.Errorf("failed to lookup node for file ID: %v", err)
	}
	err = call(targetAddress, "Put", &pb.PutRequest{
		Key:   filename,
		Value: fileData,
	}, &pb.PutResponse{})
	if err != nil {
		return fmt.Errorf("failed to store file on target node: %v", err)
	}
	//but on all its sucessors to, it -r is 3, put on 3 successors
	var resp pb.GetPredecessorResponse
	err2 := call(targetAddress, "GetPredecessor", &pb.GetPredecessorRequest{}, &resp)
	if err2 != nil {
		return fmt.Errorf("failed to get predecessor of target node: %v", err2)
	}
	for _, succ := range resp.Successors {
		if succ == "" || succ == targetAddress {
			continue
		}
		err = call(succ, "Put", &pb.PutRequest{
			Key:   filename,
			Value: fileData,
		}, &pb.PutResponse{})
		if err != nil {
			log.Printf("warning: failed to store file on successor %s: %v", succ, err)
		}
	}
	//log.Printf("StoreFile: stored file %s on node %s", filename, targetAddress)
	return nil

}

func (n *Node) checkPredecessor() {
	n.mu.RLock()
	pred := n.Predecessor
	n.mu.RUnlock()
	if pred == "" {
		return
	}
	err := call(pred, "Ping", &pb.PingRequest{}, &pb.PingResponse{})
	if err != nil {
		log.Printf("error ping: %s", err)
		n.mu.Lock()
		n.Predecessor = ""
		n.mu.Unlock()
	}
}
func (n *Node) create() {
	n.mu.Lock()
	n.Predecessor = ""
	n.Successors = []string{n.Address}
	n.mu.Unlock()
	//log.Printf("create: created new Chord network at %s", n.Address)
}

func (n *Node) join(nprime string, id string) {
	var resp pb.FindSuccessorRespons

	if id != "" {
		d := new(big.Int)
		d.SetString(id, 16)
		err := call(nprime, "FindSuccessor", &pb.FindSuccessorRequest{Id: d.Bytes()}, &resp)
		if err != nil {
			log.Printf("join: FindSuccessor call failed: %v", err)
			return
		}
	} else {
		err := call(nprime, "FindSuccessor", &pb.FindSuccessorRequest{Id: hash(n.Address).Bytes()}, &resp)
		if err != nil {
			log.Printf("join: FindSuccessor call failed: %v", err)
			return
		}
	}

	n.mu.Lock()
	n.Successors = []string{resp.Adress}
	n.mu.Unlock()
	errr := call(resp.Adress, "Notify", &pb.NotifyRequest{Address: n.Address}, &pb.NotifyResponse{})
	if errr != nil {
		log.Printf("join: Notify call failed: %v", errr)
		return
	}
	log.Printf("join: joined the network via %s, my successor is %s", n.Successors[0], resp.Adress)
}
func (n *Node) FindSuccessor(ctx context.Context, req *pb.FindSuccessorRequest) (*pb.FindSuccessorRespons, error) {
	targetId := new(big.Int).SetBytes(req.Id)

	n.mu.RLock()
	if len(n.Successors) == 0 || n.Successors[0] == "" {
		n.mu.RUnlock()
		return &pb.FindSuccessorRespons{Adress: n.Address}, nil
	}
	myHash := hash(n.Address)
	succHash := hash(n.Successors[0])
	succ := n.Successors[0]
	n.mu.RUnlock()

	// If target is between me and my successor, return my successor
	if between(myHash, targetId, succHash, true) {
		return &pb.FindSuccessorRespons{Adress: succ}, nil
	}

	// Otherwise, forward to closest preceding node
	// (For now, just forward to successor - can optimize with finger table later)

	var resp pb.FindSuccessorRespons
	err := call(succ, "FindSuccessor", req, &resp)
	if err != nil {
		//log.Printf("FindSuccessor: call to %s failed: %v", succ, err)
		return nil, err
	}

	return &resp, nil
}
func (n *Node) stabilize() {
	////log.Printf("stabilize: checking successor %s", n.Successors[0])
	n.mu.RLock()
	succ := n.Successors[0]
	pred := n.Predecessor
	n.mu.RUnlock()
	//n.dump()

	if n.Address == succ {
		if pred != "" {

			var resp pb.NotifyResponse
			err := call(pred, "Notify", &pb.NotifyRequest{Address: n.Address}, &resp)
			if err != nil {
				log.Printf("stabilize: predecessor %s is dead, clearing it", n.Predecessor)
				n.mu.Lock()
				n.Predecessor = ""
				n.mu.Unlock()
				return
			}
			n.mu.Lock()
			n.Successors[0] = n.Predecessor
			n.Successors = append(n.Successors, succ)
			n.mu.Unlock()
			//log.Printf("stabilize: self succ now got another succ %s", n.Predecessor)

		}
		return
	}
	//succ = resolveAddress(succ)

	for true {
		// ask successor for its predecessor
		var resp pb.GetPredecessorResponse
		for i := 0; i < len(n.Successors); i++ {
			if succ == n.Address {
				return
			}
			err := call(succ, "GetPredecessor", &pb.GetPredecessorRequest{}, &resp)
			if err != nil {
				//log.Printf("stabilize: GetPredecessor call failed: %v", err)
				n.mu.Lock()
				if len(n.Successors) == 0 {
					n.mu.Unlock()
					return
				}
				n.Successors = n.Successors[1:]

				if i == len(n.Successors) {
					n.mu.Unlock()
					//log.Printf("Found no alive successors")
					return
				}
				succ = n.Successors[0]

				n.mu.Unlock()

			} else {
				break
			}

		}

		//log.Printf("stabilize: complete successor list [%d entries]: %v", len(n.Successors), n.Successors)

		//log.Printf("stabilize: got predecessor %s from %s", resp.Address, succ)
		if resp.Address == "" {
			//log.Printf("stabilize: got empty predecessor from ", succ)
			if succ == n.Address || resp.Pred == n.Address {
				break
			}
			var resp pb.NotifyResponse
			err := call(succ, "Notify", &pb.NotifyRequest{Address: n.Address}, &resp)
			if err != nil {
				//log.Printf("stabilize: notify call failed: %v", err)
			}

			break
		}
		if resp.Address == n.Address {
			n.mu.Lock()
			//n.Successors[0] = succ
			n.Successors = append([]string{succ}, resp.Successors...)
			for i, addr := range n.Successors {
				if addr == n.Address {
					n.Successors = n.Successors[:i+1]
					break
				}
			}
			if len(n.Successors) > n.SuccessorListSize {
				n.Successors = n.Successors[:n.SuccessorListSize]
			}
			n.mu.Unlock()
			//log.Printf("stabilize: successor is %s", succ)
			break
		} else {
			succ = resp.Address
			//log.Printf("stabilize: trying with new successor %s", succ)
		}
	}
	////log.Printf("stab end")

}
func (n *Node) Notify(ctx context.Context, req *pb.NotifyRequest) (*pb.NotifyResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	//log.Printf("notify: received notification from %s", req.Address)
	if n.Predecessor != req.Address && req.Address != "" {
		log.Printf("notify: updating predecessor from %s to %s", n.Predecessor, req.Address)
		n.Predecessor = req.Address
	}

	return &pb.NotifyResponse{}, nil
}

// returns ip of the file
func (n *Node) Lookup(filename string) (*big.Int, string, []byte, error) { //node’s identifier, IP address, port, and the contents of the file.
	key := hash(filename)
	n.mu.RLock()
	closestNode := n.Address
	for i := keySize; i >= 1; i-- {
		if n.FingerTable[i] != "" {
			fingerHash := hash(n.FingerTable[i])
			myHash := hash(n.Address)

			// If this finger is between me and the key, use it
			if between(myHash, fingerHash, key, false) {
				closestNode = n.FingerTable[i]
				break
			}
		}
	}
	n.mu.RUnlock()
	var resp pb.FindSuccessorRespons
	err := call(closestNode, "FindSuccessor", &pb.FindSuccessorRequest{Id: key.Bytes()}, &resp)
	if err != nil {
		log.Printf("Lookup: FindSuccessor call failed: %v", err)
		return nil, "", nil, err
	}
	var getresp pb.GetResponse
	err2 := call(resp.Adress, "Get", &pb.GetRequest{Key: filename}, &getresp)
	if err2 != nil {
		log.Printf("Lookup: Get call failed: %v", err2)
		return nil, "", nil, err2
	}

	return key, resp.Adress, []byte(getresp.Value), nil
}

func call(address string, method string, request interface{}, reply interface{}) error {
	creds, err := credentials.NewClientTLSFromFile("certs/ca-cert.pem", "")
	if err != nil {
		return fmt.Errorf("failed to load TLS credentials: %v", err)
	}
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(creds))
	if err != nil {
		return err
	}
	defer conn.Close()
	client := pb.NewChordClient(conn)
	//log.Printf("call: calling %s on %s", method, address)
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
		//log.Printf("call: in Notify case")
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
	case "Ping":
		req, ok := request.(*pb.PingRequest)
		if !ok {
			return fmt.Errorf("invalid request type for Ping")
		}
		resp, err := client.Ping(context.Background(), req)
		if err != nil {
			return err
		}
		r, ok := reply.(*pb.PingResponse)
		if !ok {
			return fmt.Errorf("invalid reply type for Ping")
		}
		*r = *resp
	case "FindSuccessor":
		req, ok := request.(*pb.FindSuccessorRequest)
		if !ok {
			return fmt.Errorf("invalid request type for FindSuccessor")
		}
		resp, err := client.FindSuccessor(context.Background(), req)
		if err != nil {
			return err
		}
		r, ok := reply.(*pb.FindSuccessorRespons)
		if !ok {
			return fmt.Errorf("invalid reply type for FindSuccessor")
		}
		*r = *resp
	case "Get":
		req, ok := request.(*pb.GetRequest)
		if !ok {
			return fmt.Errorf("invalid request type for Get")
		}
		resp, err := client.Get(context.Background(), req)
		if err != nil {
			return err
		}
		r, ok := reply.(*pb.GetResponse)
		if !ok {
			return fmt.Errorf("invalid reply type for Get")
		}
		*r = *resp
	case "Put":
		req, ok := request.(*pb.PutRequest)
		if !ok {
			return fmt.Errorf("invalid request type for Put")
		}
		resp, err := client.Put(context.Background(), req)
		if err != nil {
			return err
		}
		r, ok := reply.(*pb.PutResponse)
		if !ok {
			return fmt.Errorf("invalid reply type for Put")
		}
		*r = *resp
	default:
		return fmt.Errorf("unknown method: %s", method)
	}
	//log.Printf("call: completed %s on %s", method, address)
	return nil
}

// GetPredecessor implements the GetPredecessor RPC method
func (n *Node) GetPredecessor(ctx context.Context, req *pb.GetPredecessorRequest) (*pb.GetPredecessorResponse, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return &pb.GetPredecessorResponse{Address: n.Predecessor, Successors: n.Successors, Pred: n.Predecessor}, nil
}

func (n *Node) fixFingers(nextFinger int) int {
	nextFinger = (nextFinger % keySize) + 1
	n.mu.RLock()
	hasSuccessor := len(n.Successors) > 0 && (n.Successors[0] != "" && n.Address != n.Successors[0])
	n.mu.RUnlock()

	if !hasSuccessor {
	}
	// Calculate the target position for this finger
	target := jump(n.Address, nextFinger)

	// Find the successor of that position using your own FindSuccessor

	var resp pb.FindSuccessorRespons
	err := call(n.Address, "FindSuccessor", &pb.FindSuccessorRequest{Id: target.Bytes()}, &resp)
	if err != nil {
		log.Printf("fixFingers: FindSuccessor failed for finger %d: %v", nextFinger, err)
		return nextFinger - 1
	}

	// Update the finger table entry
	n.mu.Lock()
	n.FingerTable[nextFinger] = resp.Adress
	n.mu.Unlock()

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
