package main

type StatusType int

const (
	STATUS_OK              StatusType = iota
	STATUS_NOT_INITIALIZED StatusType = iota
	STATUS_OTHER           StatusType = 9999
)

type ApiResponse struct {
	Status StatusType  `json:"status"`
	Error  *string     `json:"error"`
	Result interface{} `json:"result"`
}

type FeeType string

const (
	LASTBLOCK FeeType = "Last Block"
	MEMPOOL   FeeType = "Mempool"
	MIN_FEE   FeeType = "Default Fee"
)

type FeeValueType struct {
	Value float64 `json:"value"`
	Type  FeeType `json:"type"`
}

type TicketProfitability struct {
	MinFee  FeeValueType `json:"minFee"`
	MeanFee FeeValueType `json:"meanFee"`

	ProfitabilityMean float64 `json:"profitabilityMean"`
	ProfitabilityMin  float64 `json:"profitabilityMin"`

	TicketPrice float64 `json:"ticketPrice"`
}
