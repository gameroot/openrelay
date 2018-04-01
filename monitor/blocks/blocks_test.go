package blocks_test


import (
	"testing"
	"math/big"
	"time"
	"encoding/json"
	"github.com/notegio/openrelay/monitor/blocks"
	"github.com/notegio/openrelay/channels"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common"
	"log"
	"reflect"
)


func generateBlockHeader(parentHash common.Hash, number int64, topics []common.Hash) (*types.Header){
	bloom := types.Bloom{}
	for _, topic := range topics {
		bloom.Add(new(big.Int).SetBytes(topic[:]))
	}
	return &types.Header{
		ParentHash: parentHash,
		UncleHash: common.Hash{},
		Coinbase: common.Address{},
		Root: common.Hash{},
		TxHash: common.Hash{},
		ReceiptHash: common.Hash{},
		Bloom: bloom,
		Difficulty: new(big.Int),
		Number: big.NewInt(number),
		GasLimit: new(big.Int),
		GasUsed: new(big.Int),
		Time: big.NewInt(number),
		Extra: []byte{},
		MixDigest: common.Hash{},
		Nonce: types.BlockNonce{},
	}
}

func generateHeaderChain(n int64) []*types.Header {
	return generateChainSplit(0, n, common.Hash{}, []byte{})
}

func generateChainSplit(start, n int64, parentHash common.Hash, extra []byte) []*types.Header {
	header := generateBlockHeader(parentHash, start, []common.Hash{})
	header.Extra = extra
	headers := []*types.Header{header}
	for i := start + 1; i < start + n; i++ {
		header = generateBlockHeader(header.Hash(), int64(i), []common.Hash{})
		headers = append(headers, header)
	}
	return headers
}

type testConsumer struct {
	channel chan string
}

func (consumer *testConsumer) Consume(msg channels.Delivery) {
	consumer.channel <- msg.Payload()
}

func newTestConsumer() *testConsumer {
	return &testConsumer{make(chan string, 5)}
}

func TestGenerateBlockHeader(t *testing.T) {
	header := generateBlockHeader(common.Hash{}, 0, []common.Hash{})
	if size := len(header.Hash()); size != 32 {
		t.Errorf("Expected header hash of length 32, got %v", size)
	}
}

func TestGenerateHeaderChain(t *testing.T) {
	headers := generateHeaderChain(10)
	if size := len(headers); size != 10 {
		t.Errorf("Expected 10 headers, got %v", size)
	}
	if size := len(headers[0].Hash()); size != 32 {
		t.Errorf("Expected header hash of length 32, got %v", size)
	}
	for i := 1; i < len(headers); i++ {
		if !reflect.DeepEqual(headers[i].ParentHash, headers[i-1].Hash()) {
			t.Errorf("Parent header does not match: %#x != %#x", headers[i].ParentHash, headers[i-1].Hash())
		}
	}
}

func TestPublishBlock(t *testing.T) {
	log.Printf("TestPublishBlock")
	publisher, consumerChannel := channels.MockChannel()
	headers := generateHeaderChain(3)
	headerGetter := blocks.NewMockHeaderGetter(headers)
	blockRecorder := blocks.NewMockBlockRecorder()
	blockRecorder.Record(big.NewInt(0))
	blockMonitor := blocks.NewBlockMonitor(headerGetter, publisher, 1 * time.Second, blockRecorder, 128)
	testConsumer := newTestConsumer()
	consumerChannel.AddConsumer(testConsumer)
	consumerChannel.StartConsuming()
	go blockMonitor.Process()
	for _, header := range headers {
		payload := <-testConsumer.channel
		miniBlock := &blocks.MiniBlock{}
		if err := json.Unmarshal([]byte(payload), miniBlock); err != nil {
			t.Errorf(err.Error())
		}
		if !reflect.DeepEqual(miniBlock.Hash, header.Hash()) {
			t.Errorf("Hashes do not match")
		}
		if miniBlock.Number.Cmp(header.Number) != 0 {
			t.Errorf("Block numbers do not match")
		}
		if !reflect.DeepEqual(miniBlock.Bloom, header.Bloom) {
			t.Errorf("Bloom filters do not match")
		}
	}
	blockMonitor.Stop()
}

func TestPublishBlockAdd(t *testing.T) {
	log.Printf("TestPublishBlockAdd")
	publisher, consumerChannel := channels.MockChannel()
	headers := generateHeaderChain(4)
	headerGetter := blocks.NewMockHeaderGetter(headers[:3])
	blockRecorder := blocks.NewMockBlockRecorder()
	blockRecorder.Record(big.NewInt(0))
	blockMonitor := blocks.NewBlockMonitor(headerGetter, publisher, 1 * time.Second, blockRecorder, 128)
	testConsumer := newTestConsumer()
	consumerChannel.AddConsumer(testConsumer)
	consumerChannel.StartConsuming()
	go blockMonitor.Process()
	for _, header := range headers[:3] {
		payload := <-testConsumer.channel
		miniBlock := &blocks.MiniBlock{}
		if err := json.Unmarshal([]byte(payload), miniBlock); err != nil {
			t.Errorf(err.Error())
		}
		if !reflect.DeepEqual(miniBlock.Hash, header.Hash()) {
			t.Errorf("Hashes do not match")
		}
		if miniBlock.Number.Cmp(header.Number) != 0 {
			t.Errorf("Block numbers do not match")
		}
		if !reflect.DeepEqual(miniBlock.Bloom, header.Bloom) {
			t.Errorf("Bloom filters do not match")
		}
	}
	select {
	case _ = <-testConsumer.channel:
		t.Errorf("Got an unexpected value");
	default:
	}
	headerGetter.AddHeader(headers[3]);
	payload := <- testConsumer.channel;
	miniBlock := &blocks.MiniBlock{}
	if err := json.Unmarshal([]byte(payload), miniBlock); err != nil {
		t.Errorf(err.Error())
	}
	if !reflect.DeepEqual(miniBlock.Hash, headers[3].Hash()) {
		t.Errorf("Hashes do not match")
	}
	if miniBlock.Number.Cmp(headers[3].Number) != 0 {
		t.Errorf("Block numbers do not match")
	}
	if !reflect.DeepEqual(miniBlock.Bloom, headers[3].Bloom) {
		t.Errorf("Bloom filters do not match")
	}
	blockMonitor.Stop()
}

func TestPublishBlockReorg(t *testing.T) {
	log.Printf("TestPublishBlockReorg")
	publisher, consumerChannel := channels.MockChannel()
	headers := generateHeaderChain(3)
	headerGetter := blocks.NewMockHeaderGetter(headers)
	blockRecorder := blocks.NewMockBlockRecorder()
	blockRecorder.Record(big.NewInt(0))
	blockMonitor := blocks.NewBlockMonitor(headerGetter, publisher, 1 * time.Second, blockRecorder, 128)
	testConsumer := newTestConsumer()
	consumerChannel.AddConsumer(testConsumer)
	consumerChannel.StartConsuming()
	go blockMonitor.Process()
	for _, header := range headers {
		payload := <-testConsumer.channel
		miniBlock := &blocks.MiniBlock{}
		if err := json.Unmarshal([]byte(payload), miniBlock); err != nil {
			t.Errorf(err.Error())
		}
		if !reflect.DeepEqual(miniBlock.Hash, header.Hash()) {
			t.Errorf("Hashes do not match")
		}
		if miniBlock.Number.Cmp(header.Number) != 0 {
			t.Errorf("Block numbers do not match")
		}
		if !reflect.DeepEqual(miniBlock.Bloom, header.Bloom) {
			t.Errorf("Bloom filters do not match")
		}
	}
	reorg := generateChainSplit(1, 3, headers[1].ParentHash, []byte("split"))
	for _, header := range reorg {
		headerGetter.AddHeader(header)
	}
	for _, header := range reorg {
		payload := <-testConsumer.channel
		miniBlock := &blocks.MiniBlock{}
		if err := json.Unmarshal([]byte(payload), miniBlock); err != nil {
			t.Errorf(err.Error())
		}
		if !reflect.DeepEqual(miniBlock.Hash, header.Hash()) {
			t.Errorf("Hashes do not match")
		}
		if miniBlock.Number.Cmp(header.Number) != 0 {
			t.Errorf("Block numbers do not match")
		}
		if !reflect.DeepEqual(miniBlock.Bloom, header.Bloom) {
			t.Errorf("Bloom filters do not match")
		}
	}

	blockMonitor.Stop()
}
