package msgs

import (
	"./elevio/elevio"
	"./fsm"
)

type Direction elevio.MotorDirection

type ElevatorStatus fsm.Elevator

type OrderType elevio.ButtonType

type ElevatorState fsm.State

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

type Debug_placeOrderMsg PlacedOrderMsg
type Debug_acceptOrderMsg SafeOrderMsg

type Heartbeat struct {
	SenderID       string         `json:"sender_id"`
	ElevatorStatus ElevatorStatus `json:"elevator_status"`
	AcceptedOrders []Order        `json:"accepted_orders"`
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
	switch t {
	case elevio.BT_HallUp:
		return "Hall ↑"
	case elevio.BT_HallDown:
		return "Hall ↓"
	case elevio.BT_Cab:
		return "Cab"
	}
}
