package proxy

import (
	"log"
	"math/big"
	"strconv"
	"strings"
	"sync"

	"../rpc"
	"../util"

	"github.com/ethereum/go-ethereum/common"
)

const maxBacklog = 3

type BlockTemplate struct {
	sync.RWMutex
	Header               string
	Seed                 string
	Target               string
	Difficulty           *big.Int
	Height               uint64
	GetPendingBlockCache *rpc.GetBlockReplyPart
	nonces               map[string]bool
	headers              map[string]uint64
}

func (t *BlockTemplate) submit(nonce string) bool {
	t.Lock()
	defer t.Unlock()
	_, exist := t.nonces[nonce]
	if exist {
		return true
	}
	t.nonces[nonce] = true
	return false
}

type Block struct {
	difficulty  *big.Int
	hashNoNonce common.Hash
	nonce       uint64
	mixDigest   common.Hash
	number      uint64
}

func (b Block) Difficulty() *big.Int     { return b.difficulty }
func (b Block) HashNoNonce() common.Hash { return b.hashNoNonce }
func (b Block) Nonce() uint64            { return b.nonce }
func (b Block) MixDigest() common.Hash   { return b.mixDigest }
func (b Block) NumberU64() uint64        { return b.number }

func (s *ProxyServer) fetchBlockTemplate() {
	rpc := s.rpc()
	t := s.currentBlockTemplate()
	pendingReply, height, diff, err := s.fetchPendingBlock()
	if err != nil {
		log.Printf("Error while refreshing pending block on %s: %s", rpc.Name, err)
		return
	}
	reply, err := rpc.GetWork()
	if err != nil {
		log.Printf("Error while refreshing block template on %s: %s", rpc.Name, err)
		return
	}
	// No need to update, we have fresh job
	if t != nil && t.Header == reply[0] {
		return
	}

	pendingReply.Difficulty = util.ToHex(s.config.Proxy.Difficulty)

	newTemplate := BlockTemplate{
		Header:               reply[0],
		Seed:                 reply[1],
		Target:               reply[2],
		Height:               height,
		Difficulty:           big.NewInt(diff),
		GetPendingBlockCache: pendingReply,
		nonces:               make(map[string]bool),
		headers:              make(map[string]uint64),
	}
	// Copy headers backlog and add current one
	newTemplate.headers[reply[0]] = height
	if t != nil {
		for k, v := range t.headers {
			if v >= height-maxBacklog {
				newTemplate.headers[k] = v
			}
		}
	}
	s.blockTemplate.Store(&newTemplate)
	log.Printf("New block to mine on %s at height: %d / %s", rpc.Name, height, reply[0][0:10])
}

func (s *ProxyServer) fetchPendingBlock() (*rpc.GetBlockReplyPart, uint64, int64, error) {
	rpc := s.rpc()
	reply, err := rpc.GetPendingBlock()
	if err != nil {
		log.Printf("Error while refreshing pending block on %s: %s", rpc.Name, err)
		return nil, 0, 0, err
	}
	blockNumber, err := strconv.ParseUint(strings.Replace(reply.Number, "0x", "", -1), 16, 64)
	if err != nil {
		log.Println("Can't parse pending block number")
		return nil, 0, 0, err
	}
	blockDiff, err := strconv.ParseInt(strings.Replace(reply.Difficulty, "0x", "", -1), 16, 64)
	if err != nil {
		log.Println("Can't parse pending block difficulty")
		return nil, 0, 0, err
	}
	return reply, blockNumber, blockDiff, nil
}