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
	SenderID string `json:"sender_id"`
	Order    Order  `json:"order"`
}

type OrderPlacedAck struct {
	SenderID   string `json:"sender_id"`
	RecieverID string `json:"reciever_id"`
	Order      Order  `json:"order"`
	Score      int    `json:"score"`
}

type TakeOrderMsg struct {
	SenderID string `json:"sender_id"`
	CmdID    string `json:"cmd_id"` // specify the elevator that should take the order
	Order    Order  `json:"order"`
}

type TakeOrderAck struct {
	SenderID   string `json:"sender_id"`
	RecieverID string `json:"reciever_id"`
	Order      Order  `json:"order"`
}

type ElevatorStatus struct {
	ID        string    `json:"id"`
	Floor     int       `json:"floor"`
	Direction Direction `json:"direction"`
	Stopped   bool      `json:"stopped"`
}

type Heartbeat struct {
	SenderID       string         `json:"sender_id"`
	Status         ElevatorStatus `json:"elevator_status"`
	AcceptedOrders []Order        `json:"accepted_orders"`
}

type Debug_placeOrderMsg struct {
	RecieverID string `json:"reciever_id"`
	Order      Order  `json:"order"`
}

type Debug_acceptOrderMsg struct {
	RecieverID string `json:"reciever_id"`
	OrderID    int    `json:"order"`
}

// sort.Interface for heartbeat slices
type HeartbeatSlice []Heartbeat

func (h HeartbeatSlice) Len() int {
	return len(h)
}

func (h HeartbeatSlice) Less(i, j int) bool {
	return h[i].SenderID < h[j].SenderID
}

func (h HeartbeatSlice) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// sort.Interface for ElevatorStatus slices
type ElevatorStatusSlice []ElevatorStatus

func (e ElevatorStatusSlice) Len() int {
	return len(e)
}

func (e ElevatorStatusSlice) Less(i, j int) bool {
	return e[i].ID < e[j].ID
}

func (e ElevatorStatusSlice) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}
