package schemas

type CandleMessage struct {
	Type     string  `json:"type"`
	Symbol   string  `json:"symbol"`
	Interval string  `json:"interval"`
	Time     int64   `json:"time"` // unix seconds
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
	Closed   bool    `json:"closed"`
}
