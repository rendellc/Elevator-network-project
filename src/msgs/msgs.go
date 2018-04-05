package msgs

type Direction int

const (
	Up Direction = iota
	Down
)

type OrderType string

const (
	CabCall  OrderType = "cab"
	HallCall OrderType = "hall"
)

type ElevatorState int

const (
	MovingUp ElevatorState = iota
	MovingDown
	StopUp
	StopDown
)

type Order struct {
	ID        int       `json:"order_id"`
	Floor     int       `json:"floor"`
	Direction Direction `json:"direction"`
}

type OrderPlacedMsg struct {
	SenderID   string `json:"sender_id"`
	RecieverID string `json:"reciever_id"`
	Order      Order  `json:"order"`
	Priority   int    `json:"priority"`
}

type OrderPlacedAck struct {
	SenderID   string `json:"sender_id"`
	RecieverID string `json:"reciever_id"`
	Order      Order  `json:"order"`
	Score      int    `json:"score"`
}

type TakeOrderAck struct {
	SenderID   string `json:"sender_id"`
	RecieverID string `json:"reciever_id"`
	Order      Order  `json:"order"`
}

type Heartbeat struct {
	SenderID       string        `json:"sender_id"`
	ElevatorState  ElevatorState `json:"elevator_state"`
	AcceptedOrders []Order       `json:"accepted_orders"`
	TakenOrders    []Order       `json:"taken_orders"`
}

type TakeOrderMsg struct {
	Order Order  `json:"order"`
	CmdID string `json:"cmd_id"` // specify the elevator that should take the order
}
