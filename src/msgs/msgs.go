package msgs

import (
	"../elevio/elevio"
	"../fsm"
)

type Order struct {
	ID    int       `json:"order_id"`
	Floor int       `json:"floor"`
	Type  OrderType `json:"type"`
}

type OrderMsg struct {
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"reciever_id"`
	Order      Order  `json:"order"`
}

type PlacedOrderMsg OrderMsg
type PlacedOrderAck OrderMsg
type TakeOrderMsg OrderMsg
type TakeOrderAck OrderMsg
type SafeOrderMsg OrderMsg
type CompleteOrderMsg OrderMsg

type Heartbeat struct {
	SenderID       string       `json:"sender_id"`
	ElevatorStatus fsm.Elevator `json:"elevator_status"`
	AcceptedOrders []Order      `json:"accepted_orders"`
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

// nice printing
func (d elevio.MotorDirection) String() string {
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
	switch t {
	case elevio.BT_HallUp:
		return "Hall ↑"
	case elevio.BT_HallDown:
		return "Hall ↓"
	case elevio.BT_Cab:
		return "Cab"
	}
}
