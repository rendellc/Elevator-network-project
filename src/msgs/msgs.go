package msgs

import (
	"../elevio"
	"../fsm"
)

type Order struct {
	ID    int               `json:"order_id"`
	Floor int               `json:"floor"`
	Type  elevio.ButtonType `json:"button_type"`
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
	Status         fsm.Elevator `json:"elevator_status"`
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

func Equal(a, b Heartbeat) bool {
	if a.SenderID != b.SenderID {
		return false
	}
	if a.Status != b.Status {
		return false
	}
	if len(a.AcceptedOrders) != len(b.AcceptedOrders) {
		return false
	}

	for _, val_a := range a.AcceptedOrders {
		for _, val_b := range b.AcceptedOrders {
			if val_a == val_b {
				break
			}
			return false
		}
	}
	return true
}
