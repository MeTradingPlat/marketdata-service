package tastytrade

type setupMessage struct {
	Type                   string `json:"type"`
	Channel                int    `json:"channel"`
	Version                string `json:"version"`
	KeepaliveTimeout       int    `json:"keepaliveTimeout"`
	AcceptKeepaliveTimeout int    `json:"acceptKeepaliveTimeout"`
}

type authMessage struct {
	Type    string `json:"type"`
	Channel int    `json:"channel"`
	Token   string `json:"token"`
}

type keepaliveMessage struct {
	Type    string `json:"type"`
	Channel int    `json:"channel"`
}

type channelRequestMessage struct {
	Type       string            `json:"type"`
	Channel    int               `json:"channel"`
	Service    string            `json:"service"`
	Parameters map[string]string `json:"parameters"`
}

type channelCancelMessage struct {
	Type    string `json:"type"`
	Channel int    `json:"channel"`
}

type feedSetupMessage struct {
	Type                    string              `json:"type"`
	Channel                 int                 `json:"channel"`
	AcceptAggregationPeriod float64             `json:"acceptAggregationPeriod"`
	AcceptDataFormat        string              `json:"acceptDataFormat"`
	AcceptEventFields       map[string][]string `json:"acceptEventFields"`
}

type feedSubscriptionItem struct {
	Symbol   string `json:"symbol"`
	Type     string `json:"type"`
	FromTime *int64 `json:"fromTime,omitempty"`
}

type feedSubscriptionMessage struct {
	Type    string                  `json:"type"`
	Channel int                     `json:"channel"`
	Add     []feedSubscriptionItem  `json:"add,omitempty"`
	Remove  []feedSubscriptionItem  `json:"remove,omitempty"`
}

type inboundEnvelope struct {
	Type    string        `json:"type"`
	Channel int           `json:"channel"`
	Service string        `json:"service"`
	State   string        `json:"state"`
	Error   string        `json:"error"`
	Message string        `json:"message"`
	Data    []interface{} `json:"data"`
}
