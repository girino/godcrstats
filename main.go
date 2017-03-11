package main

import (
	//	"bufio"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	//	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrrpcclient"
)

type DcrConnections struct {
	DaemonConn       *dcrrpcclient.Client
	NotificationConn *dcrrpcclient.Client
	WalletConn       *dcrrpcclient.Client

	CurrentTicketStats         *TicketStats
	CurrentTicketProfitability *TicketProfitability
	CurrentHeight              uint32

	BlockReceiveChannel chan BlockNotification
}

type BlockNotification struct {
	BlockHeader  []byte
	Transactions [][]byte
}

// Exported default connection for use in
var statsConn *DcrConnections = nil

func getCert(path string) ([]byte, error) {
	//certHomeDir := dcrutil.AppDataDir(path, false)
	//return ioutil.ReadFile(filepath.Join(certHomeDir, "rpc.cert"))
	return ioutil.ReadFile(path)
}

func getConnection(config *ConnectionConfig, ntfnHandlers *dcrrpcclient.NotificationHandlers) (*dcrrpcclient.Client, error) {
	certs, err := getCert(config.CertPath)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	hostport := config.Host + ":" + strconv.Itoa(config.Port)
	if config.IsTestnet && config.Port < 19100 {
		log.Printf("Port %d does not seem to be on Testnet. Using anyway.", config.Port)
	}
	if !config.IsTestnet && config.Port > 19100 {
		log.Printf("Port %d does not seem to be on Mainnet. Using anyway.", config.Port)
	}

	connCfg := &dcrrpcclient.ConnConfig{
		Host:         hostport,
		Endpoint:     "ws",
		User:         config.User,
		Pass:         config.Password,
		Certificates: certs,
	}

	client, err := dcrrpcclient.New(connCfg, ntfnHandlers)
	if err != nil {
		log.Printf("Error conecting to client: %v ", connCfg)
		log.Fatal("Error conecting to client: ", err)
	}
	return client, err
}

func NewStatsCollectorConnections(config *Config) (*DcrConnections, error) {

	statsConn = &DcrConnections{}
	statsConn.BlockReceiveChannel = make(chan BlockNotification, 1)

	ntfnHandlers := dcrrpcclient.NotificationHandlers{
		OnBlockConnected: func(blockHeader []byte, transactions [][]byte) {
			log.Printf("received block header %v", blockHeader)
			headStruct := BlockNotification{
				BlockHeader:  blockHeader,
				Transactions: transactions,
			}
			select {
			case statsConn.BlockReceiveChannel <- headStruct: // Put 2 in the channel unless it is full
			default:
				log.Println("Channel Full, Droping Block %v", headStruct)
			}
		},
	}

	daemonConn, err := getConnection((*ConnectionConfig)(config.DaemonConfig), nil)
	if err != nil {
		log.Fatal(err)
	}
	// notifications use another connection in order to avoid deadlocks
	// the other solution would be bypassing dcrdpcclient altogether
	notificationConn, err := getConnection((*ConnectionConfig)(config.DaemonConfig), &ntfnHandlers)
	if err != nil {
		log.Fatal(err)
	}

	var walletConn *dcrrpcclient.Client = nil
	if config.WalletEnabled {
		walletConn, err = getConnection((*ConnectionConfig)(config.WalletConfig), nil)
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Println("Connected to dcrd and dcrwallet web sockets...")

	statsConn.DaemonConn = daemonConn
	statsConn.NotificationConn = notificationConn
	statsConn.WalletConn = walletConn
	return statsConn, nil
}

type TicketStatsDatum struct {
	TicketId      string
	RawTicket     *dcrjson.TxRawResult
	Amount        float64
	Fee           float64
	Size          int
	Profitability float64
}

// thats for sorting. Needs to implement interface
type TicketStatsData []TicketStatsDatum

func (slice TicketStatsData) Len() int {
	return len(slice)
}
func (slice TicketStatsData) Less(i, j int) bool {
	return slice[i].Profitability < slice[j].Profitability
}
func (slice TicketStatsData) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

type TicketStats struct {
	SortedTicketStats TicketStatsData
	Mean              float64
	Median            float64
	Quartils          []float64
	Decils            []float64
	Max               float64
}

func sumIns(ins []dcrjson.Vin) float64 {
	ret := float64(0)

	for _, in := range ins {
		ret += in.AmountIn
	}
	return ret
}

func sumOuts(outs []dcrjson.Vout) float64 {
	ret := float64(0)

	for _, out := range outs {
		ret += out.Value
	}
	return ret
}

func sumStakeSubmissions(outs []dcrjson.Vout) float64 {
	ret := float64(0)

	for _, out := range outs {
		if out.ScriptPubKey.Type == "stakesubmission" {
			ret += out.Value
		}
	}
	return ret
}

var txCache map[string]*dcrjson.TxRawResult = make(map[string]*dcrjson.TxRawResult)

func (statsConn *DcrConnections) GetCachedTx(ticket *chainhash.Hash) (*dcrjson.TxRawResult, error) {

	//	log.Printf("loading %s from cache...", ticket.String())
	tx, ok := txCache[ticket.String()]
	if !ok {
		//		log.Printf("NOT on cache.")
		var err error
		tx, err = statsConn.DaemonConn.GetRawTransactionVerbose(ticket)
		//		log.Printf("loaded %s from daemon...", ticket.String())
		if err != nil {
			log.Fatal(err)
			return nil, err
		}
		txCache[ticket.String()] = tx
		//		log.Printf("Added %s to Cache...", ticket.String())
	}

	return tx, nil
}

func getProfit(fee float64, amount float64, height int64) float64 {
	BLOCK_REWARD := 31.19582664
	reward := math.Pow(100.0/101.0, math.Ceil(float64(height)/6144.0)-1) * BLOCK_REWARD * 0.06
	//	log.Printf("reward: %f", reward)
	// but the fee only applies to 539 bytes, not the full 1024 bytes, so...
	totalCost := amount + fee
	totalReward := amount + reward
	ret := 100.0 * ((totalReward / totalCost) - 1)
	return ret
}

func (statsConn *DcrConnections) getSortedTicketStatsData() (TicketStatsData, error) {

	// current height
	height, err := statsConn.DaemonConn.GetBlockCount()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	log.Println("Listing Live Tickets Launch...")

	future := statsConn.DaemonConn.LiveTicketsAsync()

	log.Println("Listing Live Tickets Receive...")

	tickets, err := future.Receive()
	log.Println("Listing Live Tickets Received.")
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	log.Printf("Allocating %d structs...", len(tickets))

	ret := make(TicketStatsData, len(tickets))

	log.Printf("Loading RawTx for %d Tickets", len(tickets))

	txs := make([]*dcrjson.TxRawResult, len(tickets))

	for i, ticket := range tickets {
		tx, err := statsConn.GetCachedTx(ticket)
		if err != nil {
			log.Fatal(err)
		}
		txs[i] = tx
	}

	log.Printf("Processing RawTx for %d Tickets", len(tickets))
	for i, tx := range txs {
		ret[i] = TicketStatsDatum{
			TicketId:      tx.Txid,
			RawTicket:     tx,
			Amount:        sumStakeSubmissions(tx.Vout),
			Fee:           sumIns(tx.Vin) - sumOuts(tx.Vout),
			Size:          len(tx.Hex) / 2,
			Profitability: 0,
		}
		ret[i].Profitability = getProfit(ret[i].Fee, ret[i].Amount, height)

	}

	log.Println("Sorting Tickets...")
	sort.Sort(ret)

	log.Println("getStatsData Done.")

	return ret, nil
}

func (statsConn *DcrConnections) GetTicketStats() (*TicketStats, error) {
	data, err := statsConn.getSortedTicketStatsData()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	ret := TicketStats{
		SortedTicketStats: data,
		Mean:              0,
		Median:            0,
		Quartils:          make([]float64, 4),
		Decils:            make([]float64, 10),
		Max:               0,
	}
	sum := 0.0
	for _, datum := range ret.SortedTicketStats {
		sum += datum.Profitability
	}
	ret.Mean = sum / float64(len(ret.SortedTicketStats))
	ret.Median = ret.SortedTicketStats[len(ret.SortedTicketStats)/2].Profitability
	for i, _ := range ret.Quartils {
		ret.Quartils[i] = ret.SortedTicketStats[len(ret.SortedTicketStats)/4*i].Profitability
	}
	for i, _ := range ret.Decils {
		ret.Decils[i] = ret.SortedTicketStats[len(ret.SortedTicketStats)/10*i].Profitability
	}
	ret.Max = ret.SortedTicketStats[len(ret.SortedTicketStats)-1].Profitability

	ret.SortedTicketStats = nil

	return &ret, nil
}

func (statsConn *DcrConnections) GetCurrentProfitability() (ret *TicketProfitability, err error) {

	// current height
	height, err := statsConn.DaemonConn.GetBlockCount()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	// get stake diff
	diff, err := statsConn.DaemonConn.GetStakeDifficulty()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	// estimate fees
	var blocks uint32 = 1
	feeinfo, err := statsConn.DaemonConn.TicketFeeInfo(&blocks, nil)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	// new return struct
	ret = &TicketProfitability{
		MinFee:      FeeValueType{Value: 0.01, Type: MIN_FEE},
		MeanFee:     FeeValueType{Value: 0.01, Type: MIN_FEE},
		TicketPrice: diff.CurrentStakeDifficulty,
	}

	if feeinfo.FeeInfoMempool.Number >= 20 || feeinfo.FeeInfoBlocks[0].Number >= 20 {
		ret.MinFee = FeeValueType{Value: feeinfo.FeeInfoBlocks[0].Min,
			Type: LASTBLOCK}
		if feeinfo.FeeInfoMempool.Min > ret.MinFee.Value {
			ret.MinFee = FeeValueType{Value: feeinfo.FeeInfoMempool.Min,
				Type: MEMPOOL}
		}
		ret.MeanFee = FeeValueType{Value: feeinfo.FeeInfoBlocks[0].Mean,
			Type: LASTBLOCK}
		if feeinfo.FeeInfoMempool.Mean > ret.MeanFee.Value {
			ret.MinFee = FeeValueType{Value: feeinfo.FeeInfoMempool.Mean,
				Type: MEMPOOL}
		}
	}

	ret.ProfitabilityMin = getProfit(ret.MinFee.Value, diff.CurrentStakeDifficulty, height)
	ret.ProfitabilityMean = getProfit(ret.MeanFee.Value, diff.CurrentStakeDifficulty, height)

	return ret, nil
}

func (statsConn *DcrConnections) UpdateStats() {
	stats, err := statsConn.GetTicketStats()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Stats Update %v", stats)
	profit, err := statsConn.GetCurrentProfitability()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("profit: %v", profit)
	statsConn.CurrentTicketStats = stats
	statsConn.CurrentTicketProfitability = profit
	log.Printf("All data: %+v", statsConn)
}

func (statsConn *DcrConnections) NotificationChanListener() {

	for true {
		log.Println("Waiting for header...")
		blockNotification := <-statsConn.BlockReceiveChannel
		log.Println("Desserializing header...")
		blockHeader := &wire.BlockHeader{}
		err := blockHeader.FromBytes(blockNotification.BlockHeader)
		if err != nil {
			log.Fatalf("Could not desserialize %v", blockNotification.BlockHeader)
		}
		log.Printf("Desserialized header: %+v", blockHeader)
		if blockHeader.Height >= statsConn.CurrentHeight {
			log.Println("Calling UpdateStats()...")
			statsConn.CurrentHeight = blockHeader.Height
			statsConn.UpdateStats()
		}
	}

}

func main() {

	//dcrrpcclient.SetLogWriter(bufio.NewWriter(os.Stderr), "trace")

	config = NewConfigFromFile("config.json")

	var err error
	statsConn, err = NewStatsCollectorConnections(config)
	if err != nil {
		log.Fatal(err)
	}

	h, err := statsConn.DaemonConn.GetBestBlockHash()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Gote Best Block hash: %v", h)
	best, err := statsConn.DaemonConn.GetBlock(h)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Gote Best Block: %v", best)
	header, err := best.BlockHeaderBytes()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Gote Best Block header: %v", header)

	statsConn.BlockReceiveChannel <- BlockNotification{
		BlockHeader: header,
	}

	err = statsConn.NotificationConn.NotifyBlocks()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Ebabled Notify Blocks")

	var wg sync.WaitGroup

	wg.Add(1)
	// listen for notifications
	go statsConn.NotificationChanListener()

	log.Printf("HTTP server listening on %s.", config.HttpServerListen)
	router := NewRouter()
	log.Fatal(http.ListenAndServe(config.HttpServerListen, router))

	wg.Wait()

	statsConn.DaemonConn.Shutdown()
	statsConn.DaemonConn.WaitForShutdown()
}
