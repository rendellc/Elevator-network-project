package msgs

import(
	"./elevio/elevio"
	"./fsm"
)


type Direction elevio.MotorDirection

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

type OrderMsg struct {
	SenderID   string `json:"sender_id"`
	RecieverID string `json:"reciever_id"`
	Order      Order  `json:"order"`
}

type PlaceOrderMsg OrderMsg
type OrderPlacedAck OrderMsg
type TakeOrderMsg OrderMsg
type TakeOrderAck OrderMsg
type SafeOrderMsg OrderMsg
type CompleteOrderMsg OrderMsg

type Debug_placeOrderMsg PlaceOrderMsg
type Debug_acceptOrderMsg SafeOrderMsg

type ElevatorStatus struct {
	ID string 												`json:"id"`
	Floor int													`json:"floor"`
	Dir Direction											`json:"direction"`
	Orders[N_FLOORS][N_BUTTONS] bool	`json:"orders"`
	State fsm.State										`json:"behaviour"`
}

type Heartbeat struct {
	SenderID       string        `json:"sender_id"`
	ElevatorState  ElevatorState `json:"elevator_state"`
	AcceptedOrders []Order       `json:"accepted_orders"`
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

// nice printing
func (d Direction) String() string {
	switch d {
	case elevio.MD_Up:
		return "↑"
	case elevio.Md_Down:
		return "↓"
	case elevio.MD_Stop:
		return "⛔"
	}
	return "-invalidDirection-"
}
func (t OrderType) String() string {
	return string(t)
}
