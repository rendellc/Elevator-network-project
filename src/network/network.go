package network

import (
	"../comm/bcast"
	"../comm/peers"
	"../msgs"
	"fmt"
	"math"
	"math/rand"
	"time"
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

//const server_ip = "129.241.187.38"
const port = 20010
const timeout = 1 * time.Second
const giveupAckwaitTimeout = 5 * time.Second

func Launch(id string,
	thisElevatorStatusCh <-chan msgs.ElevatorStatus, otherElevatorsStatusCh chan<- []msgs.ElevatorStatus, downedElevatorsCh chan<- []msgs.Heartbeat,
	placedOrderCh <-chan msgs.Order, thisTakeOrderCh chan<- msgs.TakeOrderMsg, otherTakeOrderCh <-chan msgs.TakeOrderMsg,
	safeOrderCh chan<- msgs.SafeOrderMsg, completedOrderCh <-chan msgs.Order) {

	placedOrderSendCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckSendCh := make(chan msgs.PlacedOrderAck)
	takeOrderAckSendCh := make(chan msgs.TakeOrderAck)
	takeOrderSendCh := make(chan msgs.TakeOrderMsg)
	completeOrderSendCh := make(chan msgs.CompleteOrderMsg)
	go bcast.Transmitter(port, placedOrderSendCh, placedOrderAckSendCh, takeOrderAckSendCh, takeOrderSendCh, completeOrderSendCh)

	placedOrderRecvCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckRecvCh := make(chan msgs.PlacedOrderAck)
	takeOrderAckRecvCh := make(chan msgs.TakeOrderAck)
	takeOrderRecvCh := make(chan msgs.TakeOrderMsg)
	completeOrderRecvCh := make(chan msgs.CompleteOrderMsg)
	go bcast.Receiver(port, placedOrderRecvCh, placedOrderAckRecvCh, takeOrderAckRecvCh, takeOrderRecvCh, completeOrderRecvCh)

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(port, peerTxEnable, peerStatusSendCh)

	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(port, peerUpdateCh)

	// bookkeeping variables
	ordersRecieved := make(map[int]msgs.Order)    // list of all placed orders to/from all elevators
	placeUnackedOrders := make(map[int]time.Time) // time is time when added
	takeUnackedOrders := make(map[int]time.Time)  // time is time when added
	ongoingOrders := make(map[int]time.Time)      // time is time when added

	for {
		select {
		case msg := <-placedOrderRecvCh:
			// store order
			if _, exists := ordersRecieved[msg.Order.ID]; exists {
				fmt.Printf("[placedOrderRecvCh]: Warning, order id %v already exists, new order ignored", msg.Order.ID)
				break
			}
			ordersRecieved[msg.Order.ID] = msg.Order

			if msg.SenderID != id { // ignore internal msgs
				// Order transmitted from other node

				// acknowledge order
				ack := msgs.PlacedOrderAck{SenderID: id,
					RecieverID: msg.SenderID,
					Order:      msg.Order}
				fmt.Printf("[placedOrderRecvCh]: Sending ack to %v for order %v\n", ack.RecieverID, ack.Order.ID)
				placedOrderAckSendCh <- ack
			} else {
				// This node has sent out an order. Needs to listen for acks
				if ackwait, exists := placeUnackedOrders[msg.Order.ID]; exists {
					fmt.Printf("[placedOrderRecvCh]: Warning, ack wait id %v already exists %v\n", msg.Order.ID, time.Now().Sub(ackwait))
				} else {
					placeUnackedOrders[msg.Order.ID] = time.Now()
				}
			}
		case order := <-placedOrderCh:
			placedOrderSendCh <- msgs.PlacedOrderMsg{SenderID: id, Order: order}
		case msg := <-placedOrderAckRecvCh:
			if msg.RecieverID == id { // ignore msgs to other nodes
				// Acknowledgement recieved from other node
				if _, ok := placeUnackedOrders[msg.Order.ID]; !ok {
					break // Not waiting for acknowledgment
				}

				fmt.Printf("[placedOrderAckRecvCh]: %v acknowledged\n", msg.Order.ID)
				delete(placeUnackedOrders, msg.Order.ID)

				// Order is safe since multiple elevators knows about it
				safeMsg := msgs.SafeOrderMsg{SenderID: id, RecieverID: id, Order: msg.Order}
				safeOrderCh <- safeMsg
			}
		case msg := <-otherTakeOrderCh:
			takeOrderSendCh <- msg
			takeUnackedOrders[msg.Order.ID] = time.Now()
		case msg := <-takeOrderRecvCh:
			if msg.RecieverID == id {
				thisTakeOrderCh <- msg

				ack := msgs.TakeOrderAck{SenderID: id, RecieverID: msg.SenderID, Order: msg.Order}
				takeOrderAckSendCh <- ack
			}
		case msg := <-takeOrderAckRecvCh:
			if msg.RecieverID == id {
				fmt.Printf("[takeOrderAckRecvCh]: Recieved ack: %v\n", msg)
				delete(takeUnackedOrders, msg.Order.ID)

			}

			// contains all ongoing orders from all elevators
			ongoingOrders[msg.Order.ID] = time.Now()
		case peerUpdate := <-peerUpdateCh:
			if len(peerUpdate.Lost) > 0 {
				var downedElevators []msgs.Heartbeat
				for _, lastHeartbeat := range peerUpdate.Lost {
					downedElevators = append(downedElevators, lastHeartbeat)
				}

				downedElevatorsCh <- downedElevators
			}

			if len(peerUpdate.New) > 0 {
				//fmt.Println("[peerUpdateCh]: New: ", peerUpdate.New)
			}

			otherElevatorsStatusCh <- peerUpdate.Peers
		case order := <-completedOrderCh:
			fmt.Println("[orderCompletedCh]: ", order)

			completeOrderSendCh <- msgs.CompleteOrderMsg{Order: order}
		case msg := <-completeOrderRecvCh:
			if _, exists := ongoingOrders[msg.Order.ID]; exists {
				fmt.Println("[orderCompletedRecvCh]: ", msg.Order)
				delete(ongoingOrders, msg.Order.ID)
			}
		case status := <-thisElevatorStatusCh:
			var acceptedOrders []msgs.Order
			for orderID, _ := range ongoingOrders {
				acceptedOrders = append(acceptedOrders, ordersRecieved[orderID])
			}
			heartbeat := msgs.Heartbeat{SenderID: id, Status: status, AcceptedOrders: acceptedOrders}
			peerStatusSendCh <- heartbeat
		case <-time.After(1 * time.Second):
			// an (empty) event every second, avoids some forms of locking
		}

		// actions that happen on every update
		for orderID, t := range placeUnackedOrders {
			if time.Now().Sub(t) > giveupAckwaitTimeout {
				fmt.Printf("[timeout]: place ack for %v\n", orderID)

				// TODO: should accept order if this happens many times!

				delete(placeUnackedOrders, orderID)
			}
		}

		for orderID, t := range takeUnackedOrders {
			if time.Now().Sub(t) > giveupAckwaitTimeout {
				fmt.Printf("[timeout]: take ack for %v\n", orderID)
				msg := msgs.TakeOrderMsg{SenderID: id, RecieverID: id,
					Order: ordersRecieved[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg

				delete(takeUnackedOrders, orderID)
			}
		}

		for orderID, t := range ongoingOrders {
			if time.Now().Sub(t) > 30*time.Second {
				fmt.Printf("[timeout]: complete not recieved for %v\n\t%v\n", orderID, ongoingOrders)

				msg := msgs.TakeOrderMsg{SenderID: id, RecieverID: id,
					Order: ordersRecieved[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg
				delete(ongoingOrders, orderID)
				delete(ordersRecieved, orderID)
			}
		}
	}
}

// pseudo-orderHandler and fsm
func PseudoOrderHandlerAndFsm(id string, thisElevatorStatusCh chan<- msgs.ElevatorStatus, otherElevatorsStatusCh <-chan []msgs.ElevatorStatus, downedElevatorsCh <-chan []msgs.Heartbeat,
	placedOrderCh chan<- msgs.Order, thisTakeOrderCh <-chan msgs.TakeOrderMsg, otherTakeOrderCh chan<- msgs.TakeOrderMsg,
	safeOrderCh <-chan msgs.SafeOrderMsg, completedOrderCh chan<- msgs.Order) {

	calculateOrderScore := func(status msgs.ElevatorStatus, order msgs.Order) int {
		floordiff := (int)(math.Abs((float64)(status.Floor - order.Floor)))
		samedirection := status.Direction == order.Direction

		var score int = 1
		score += floordiff
		if status.Stopped {
			score += 2
		}
		if !samedirection {
			score *= 2
		}

		return score
	}

	orders := make(map[int]msgs.Order)
	acceptedOrders := make(map[int]bool)     // used as a set
	thisElevatorOrders := make(map[int]bool) // used as a set
	thisElevatorOrdersUpdated := false

	dbg_placedOrderCh := make(chan msgs.Debug_placeOrderMsg)
	dbg_acceptOrderCh := make(chan msgs.Debug_acceptOrderMsg)
	go bcast.Receiver(port, dbg_placedOrderCh, dbg_acceptOrderCh)

	const fsmMaxFloor int = 4
	fsmStatus := msgs.ElevatorStatus{ID: id, Direction: msgs.Up, Floor: 1 + rnd.Intn(fsmMaxFloor-1)}

	fmt.Println("[fsm] started at: ", fsmStatus)
	thisElevatorStatusCh <- fsmStatus

	var elevators []msgs.ElevatorStatus

	for {
		select {
		case elevators = <-otherElevatorsStatusCh:
			fmt.Printf("[orderHandler]: elevators: ")
			for _, elevator := range elevators {
				fmt.Printf("%v ", elevator.ID)
			}
			fmt.Printf("\n")
		case downedElevators := <-downedElevatorsCh:
			for _, lastHeartbeat := range downedElevators {
				// elevator is down
				fmt.Printf("[orderHandler]: down: %+v %v\n", lastHeartbeat.SenderID, lastHeartbeat.AcceptedOrders)

				// take order this elevator had
				for _, order := range lastHeartbeat.AcceptedOrders {
					orders[order.ID] = order
					thisElevatorOrders[order.ID] = true
				}
			}
		case <-time.After(20 * time.Second):
			fmt.Println("[fsm] status: ", fsmStatus)
		case <-time.After(5 * time.Second):
			// pseudo-fsm

			// check if we can complete an order
			fsmStatus.Stopped = false
			for orderID, _ := range thisElevatorOrders {
				if orders[orderID].Floor == fsmStatus.Floor &&
					orders[orderID].Direction == fsmStatus.Direction {
					fmt.Printf("[fsm]: completing order %v\n", orderID)
					fsmStatus.Stopped = true

					// broadcast to network that order is completed
					completedOrderCh <- orders[orderID]

					// remove order from orderHandler/fsm
					delete(thisElevatorOrders, orderID)
					thisElevatorOrdersUpdated = true // for debugging
					delete(acceptedOrders, orderID)
					delete(orders, orderID)
				}
			}

			if !fsmStatus.Stopped {
				if fsmStatus.Floor == fsmMaxFloor {
					fsmStatus.Direction = msgs.Down
				} else if fsmStatus.Floor == 1 {
					fsmStatus.Direction = msgs.Up
				}
				if fsmStatus.Direction == msgs.Up {
					fsmStatus.Floor += 1
				} else if fsmStatus.Direction == msgs.Down {
					fsmStatus.Floor -= 1
				}
			}

			var acceptedOrderList []msgs.Order
			for orderID, _ := range acceptedOrders {
				if order, exists := orders[orderID]; exists {
					acceptedOrderList = append(acceptedOrderList, order)
				} else {
					fmt.Printf("[thisElevatorStatusCh]: Warn: orderID %v didn't exist")
				}
			}

			thisElevatorStatusCh <- fsmStatus
		case msg := <-thisTakeOrderCh:
			if _, exists := orders[msg.Order.ID]; !exists {
				fmt.Printf("[thisTakeOrderCh]: didnt have order %v,from before, %v\n", msg.Order.ID, orders)
				orders[msg.Order.ID] = msg.Order
			}
			// error checking
			if orders[msg.Order.ID] != msg.Order {
				fmt.Printf("[thisTakeOrderCh]: had different order with same ID \n\t(my)%+v\n\t(recv)%+v\n", orders[msg.Order.ID], msg.Order)
			}

			acceptedOrders[msg.Order.ID] = true
			thisElevatorOrders[msg.Order.ID] = true
			thisElevatorOrdersUpdated = true // for debugging
		case safeMsg := <-safeOrderCh:
			fmt.Printf("[safeOrderCh]: %v\n", safeMsg)
			if safeMsg.RecieverID == id {
				if _, exists := orders[safeMsg.Order.ID]; exists {
					acceptedOrders[safeMsg.Order.ID] = true

					scoreMap := make(map[string]int)
					for _, elevator := range elevators {
						scoreMap[elevator.ID] = calculateOrderScore(elevator, orders[safeMsg.Order.ID])
					}

					// find best (lowest) score
					bestID := id
					for id, score := range scoreMap {
						if score < scoreMap[bestID] {
							bestID = id
						}
					}

					fmt.Printf("[orderHandler]: elevator %v should take order %v (%v)\n", bestID, safeMsg.Order.ID, scoreMap)
					if bestID != id {
						takeOrderMsg := msgs.TakeOrderMsg{SenderID: id, RecieverID: bestID, Order: orders[safeMsg.Order.ID]}
						otherTakeOrderCh <- takeOrderMsg
					} else {
						thisElevatorOrders[safeMsg.Order.ID] = true
						thisElevatorOrdersUpdated = true // for debugging
					}
				} else {
					fmt.Println("[orderHandler]: order didn't exist")
				}
			}
		}

		if thisElevatorOrdersUpdated {
			thisElevatorOrdersUpdated = false
			fmt.Printf("[orderhandler]: my orders")
			for orderID, _ := range thisElevatorOrders {
				fmt.Printf(" %v", orderID)
			}
			fmt.Printf("\n")
		}
	}
}
