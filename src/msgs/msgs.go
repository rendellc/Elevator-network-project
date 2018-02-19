package msgs

type Direction int

const (
	Up Direction = iota
	Down
)

type ElevatorState int

const (
	MovingUp ElevatorState = iota
	MovingDown
	StopUp
	StopDown
)

type Order struct {
	OrderID   int       `json:"order_id"`
	Floor     int       `json:"floor"`
	Direction Direction `json:"direction"`
}

type OrderPlacedMsg struct {
	SourceID int    `json:"source_id"`
	MsgType  string `json:"msg_type"`
	Order    Order  `json:"order"`
	Priority int    `json:"priority"`
}

type OrderPlacedAck struct {
	SourceID int    `json:"source_id"`
	OrderID  int    `json:"order_id"`
	MsgType  string `json:"msg_type"`
	Score    int    `json:"score"`
}

type TakeOrderAck struct {
	OrderID int    `json:"order_id"`
	MsgType string `json:"msg_type"`
}

type Heartbeat struct {
	SourceID       string        `json:"source_id"`
	ElevatorState  ElevatorState `json:"elevator_state"`
	AcceptedOrders []Order       `json:"accepted_orders"`
	TakenOrders    []Order       `json:"taken_orders"`
}

type TakeOrderMsg struct {
	Order Order `json:"order"`
	CmdID int   `json:"cmd_id"` // specify the elevator that should take the order
}
