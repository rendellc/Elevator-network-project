package main

import (
	"./comm/bcast"
	"./comm/peers"
	"./msgs"
	"./fsm"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"time"
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

//var rnd = rand.New(rand.NewSource(0))

//const server_ip = "129.241.187.38"
const port = 20010
const timeout = 1 * time.Second
const giveupAckwaitTimeout = 5 * time.Second

var id_ptr = flag.String("id", "noid", "ID for node")

func main() {
	flag.Parse()

	orderPlacedSendCh := make(chan msgs.PlaceOrderMsg)
	orderPlacedAckSendCh := make(chan msgs.OrderPlacedAck)
	takeOrderAckSendCh := make(chan msgs.TakeOrderAck)
	takeOrderSendCh := make(chan msgs.TakeOrderMsg)
	completeOrderSendCh := make(chan msgs.CompleteOrderMsg)
	go bcast.Transmitter(port, orderPlacedSendCh, orderPlacedAckSendCh, takeOrderAckSendCh, takeOrderSendCh, completeOrderSendCh)

	orderPlacedRecvCh := make(chan msgs.PlaceOrderMsg)
	orderPlacedAckRecvCh := make(chan msgs.OrderPlacedAck)
	takeOrderAckRecvCh := make(chan msgs.TakeOrderAck)
	takeOrderRecvCh := make(chan msgs.TakeOrderMsg)
	completeOrderRecvCh := make(chan msgs.CompleteOrderMsg)
	go bcast.Receiver(port, orderPlacedRecvCh, orderPlacedAckRecvCh, takeOrderAckRecvCh, takeOrderRecvCh, completeOrderRecvCh)

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(port, peerTxEnable, peerStatusSendCh)

	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(port, peerUpdateCh)

	// fsm
	thisElevatorHeartbeatCh := make(chan msgs.Heartbeat)
	// OrderHandler channels
	elevatorsStatusCh := make(chan []msgs.ElevatorStatus)
	downedElevatorsCh := make(chan []msgs.Heartbeat)
	otherTakeOrderCh := make(chan msgs.TakeOrderMsg)
	thisTakeOrderCh := make(chan msgs.TakeOrderMsg)
	safeOrderCh := make(chan msgs.SafeOrderMsg)
	completedOrderCh := make(chan msgs.Order)

	// internal for network
	ordersRecieved := make(map[int]msgs.Order)
	placeUnackedOrders := make(map[int]time.Time) // time is time when added
	takeUnackedOrders := make(map[int]time.Time)  // time is time when added
	ongoingOrders := make(map[int]time.Time)      // time is time when added

	go pseudoOrderHandlerAndFsm(thisElevatorHeartbeatCh, elevatorsStatusCh, downedElevatorsCh,
		thisTakeOrderCh, otherTakeOrderCh,
		safeOrderCh, completedOrderCh)

	fmt.Println("Listening")
	for {
		select {
		case msg := <-orderPlacedRecvCh:
			// store order
			if _, exists := ordersRecieved[msg.Order.ID]; exists {
				fmt.Printf("[orderPlacedRecvCh]: Warning, order id %v already exists, new order ignored", msg.Order.ID)
				break
			}
			ordersRecieved[msg.Order.ID] = msg.Order

			if msg.SenderID != *id_ptr { // ignore internal msgs
				// Order transmitted from other node

				// acknowledge order
				ack := msgs.OrderPlacedAck{SenderID: *id_ptr,
					RecieverID: msg.SenderID,
					Order:      msg.Order}
				fmt.Printf("[orderPlacedRecvCh]: Sending ack to %v for order %v\n", ack.RecieverID, ack.Order.ID)
				orderPlacedAckSendCh <- ack
			} else {
				// This node has sent out an order. Needs to listen for acks
				if ackwait, exists := placeUnackedOrders[msg.Order.ID]; exists {
					fmt.Printf("[orderPlacedRecvCh]: Warning, ack wait id %v already exists %v\n", msg.Order.ID, time.Now().Sub(ackwait))
				} else {
					placeUnackedOrders[msg.Order.ID] = time.Now()
				}
			}
		case msg := <-orderPlacedAckRecvCh:
			if msg.RecieverID == *id_ptr { // ignore msgs to other nodes
				// Acknowledgement recieved from other node
				if _, ok := placeUnackedOrders[msg.Order.ID]; !ok {
					break // Not waiting for acknowledgment
				}

				fmt.Printf("[orderPlacedAckRecvCh]: %v acknowledged\n", msg.Order.ID)
				delete(placeUnackedOrders, msg.Order.ID)

				// Order is safe since multiple elevators know about it
				safeMsg := msgs.SafeOrderMsg{SenderID: *id_ptr, RecieverID: *id_ptr, Order: msg.Order}
				safeOrderCh <- safeMsg
			}
		case msg := <-otherTakeOrderCh:
			takeOrderSendCh <- msg
			takeUnackedOrders[msg.Order.ID] = time.Now()
		case msg := <-takeOrderRecvCh:
			if msg.RecieverID == *id_ptr {
				thisTakeOrderCh <- msg

				ack := msgs.TakeOrderAck{SenderID: *id_ptr, RecieverID: msg.SenderID, Order: msg.Order}
				takeOrderAckSendCh <- ack
			}
		case msg := <-takeOrderAckRecvCh:
			if msg.RecieverID == *id_ptr {
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

			elevatorsStatusCh <- peerUpdate.Peers
		case order := <-completedOrderCh:
			fmt.Println("[orderCompletedCh]: ", order)

			completeOrderSendCh <- msgs.CompleteOrderMsg{Order: order}
		case msg := <-completeOrderRecvCh:
			if _, exists := ongoingOrders[msg.Order.ID]; exists {
				fmt.Println("[orderCompletedRecvCh]: ", msg.Order)
				delete(ongoingOrders, msg.Order.ID)
			}
		case heartbeat := <-thisElevatorHeartbeatCh:
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
				msg := msgs.TakeOrderMsg{SenderID: *id_ptr, RecieverID: *id_ptr,
					Order: ordersRecieved[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg

				delete(takeUnackedOrders, orderID)
			}
		}

		for orderID, t := range ongoingOrders {
			if time.Now().Sub(t) > 30*time.Second {
				fmt.Printf("[timeout]: complete not recieved for %v\n\t%v\n", orderID, ongoingOrders)

				msg := msgs.TakeOrderMsg{SenderID: *id_ptr, RecieverID: *id_ptr,
					Order: ordersRecieved[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg
				delete(ongoingOrders, orderID)
				delete(ordersRecieved, orderID)
			}
		}
	}
}

// pseudo-orderHandler and fsm
func pseudoOrderHandlerAndFsm(thisElevatorHeartbeatCh chan<- msgs.Heartbeat, elevatorsStatusCh <-chan []msgs.ElevatorStatus, downedElevatorsCh <-chan []msgs.Heartbeat,
	thisTakeOrderCh <-chan msgs.TakeOrderMsg, otherTakeOrderCh chan<- msgs.TakeOrderMsg,
	safeOrderCh <-chan msgs.SafeOrderMsg, completedOrderCh chan<- msgs.Order) {

		addHallOrderCh := make(chan OrderEvent)
		deleteHallOrderCh := make(chan elevio.ButtonEvent)
		placedHallOrderCh := make(chan elevio.ButtonEvent)
		completedHallOrderCh := make(chan elevio.ButtonEvent)
		elevatorStatusCh := make(chan Elevator)
		go fsm.FSM(addHallOrderCh, deleteHallOrderCh, placedHallOrderCh, completedHallOrderCh, elevatorStatusCh)
		var elevatorStatus fsm.Elevator


		orders := make(map[int]msgs.Order)	// difference orders and acceptedOrders ???
		acceptedOrders := make(map[int]bool)     // used as a set

		dbg_placeOrderCh := make(chan msgs.Debug_placeOrderMsg)
		dbg_acceptOrderCh := make(chan msgs.Debug_acceptOrderMsg)
		go bcast.Receiver(port, dbg_placeOrderCh, dbg_acceptOrderCh)

		thisElevatorOrdersUpdated := false
		fmt.Println("[fsm] started at: ", elevatorStatus)
		//thisElevatorHeartbeatCh <- msgs.Heartbeat{SenderID: *id_ptr, Status: fsmStatus, AcceptedOrders: []msgs.Order{}}

		var elevators []msgs.ElevatorStatus

		for {
			select {
			case elevators = <-elevatorsStatusCh:	// debugging. OK
				fmt.Printf("[orderHandler]: elevators: ")
				for _, elevator := range elevators {
					fmt.Printf("%v ", elevator.ID)
				}
				fmt.Printf("\n")

			case downedElevators := <-downedElevatorsCh:	// OK
				for _, lastHeartbeat := range downedElevators {
					// elevator is down
					fmt.Printf("[orderHandler]: down: %+v %v\n", lastHeartbeat.SenderID, lastHeartbeat.AcceptedOrders)
					// take order this elevator had
					for _, order := range lastHeartbeat.AcceptedOrders {
						orders[order.ID] = order
						addHallOrderCh <- fsm.OrderEvent{order.Floor, order.Type, false}	//turn on/off lights? ???
					}
				}

			case <-time.After(20 * time.Second): // debugging. OK
				fmt.Println("[fsm] status: ", elevatorStatus)

			case elevatorStatus <- elevatorStatusCh:	// Here
				var acceptedOrderList []msgs.Order
				for orderID, _ := range acceptedOrders {
					if order, exists := orders[orderID]; exists {
						acceptedOrderList = append(acceptedOrderList, order)
					} else {
						fmt.Printf("[thisElevatorHeartbeatCh]: Warn: orderID %v didn't exist")
					}
				}
				thisElevatorHeartbeatCh <- msgs.Heartbeat{SenderID: *id_ptr, ElevatorStatus: elevatorStatus, AcceptedOrders: acceptedOrderList}

			case buttonEvent <- completedHallOrderCh:	// OK
				for orderID, _ := range thisElevatorOrders {
					if orders[orderID].Floor == buttonEvent.Floor &&
						orders[orderID].Type == buttonEvent.Button {
						fmt.Printf("[fsm]: completed order %v\n", orderID)
						// broadcast to network that order is completed
						completedOrderCh <- orders[orderID]
						// remove order from orderHandler/fsm
						thisElevatorOrdersUpdated = true // for debugging
						delete(acceptedOrders, orderID)
						delete(orders, orderID)
					}
				}

			case msg := <-thisTakeOrderCh:	// OK
				if _, exists := orders[msg.Order.ID]; !exists {
					fmt.Printf("[thisTakeOrderCh]: didnt have order %v,from before, %v\n", msg.Order.ID, orders)
					orders[msg.Order.ID] = msg.Order
					addHallOrderCh <- fsm.OrderEvent{msg.Order.Floor, msg.Order.Type, true}	// turn on/off light? ???
				}
				// error checking
				if orders[msg.Order.ID] != msg.Order {
					fmt.Printf("[thisTakeOrderCh]: had different order with same ID \n\t(my)%+v\n\t(recv)%+v\n", orders[msg.Order.ID], msg.Order)
				}
				acceptedOrders[msg.Order.ID] = true
				thisElevatorOrdersUpdated = true // for debugging

			case safeMsg := <-safeOrderCh:
				fmt.Printf("[safeOrderCh]: %v\n", safeMsg)
				if safeMsg.RecieverID == *id_ptr {
					if _, exists := orders[safeMsg.Order.ID]; exists {
						acceptedOrders[safeMsg.Order.ID] = true

						scoreMap := make(map[string]int)
						for _, elevator := range elevators {
							scoreMap[elevator.ID] = calculateOrderScore(elevator, orders[safeMsg.Order.ID])
						}

						// find best (lowest) score
						bestID := *id_ptr
						for id, score := range scoreMap {
							if score < scoreMap[bestID] {
								bestID = id
							}
						}

						fmt.Printf("[orderHandler]: elevator %v should take order %v (%v)\n", bestID, safeMsg.Order.ID, scoreMap)
						if bestID != *id_ptr {
							takeOrderMsg := msgs.TakeOrderMsg{SenderID: *id_ptr, RecieverID: bestID, Order: orders[safeMsg.Order.ID]}
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

//Security copy
/*
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
*/
	orders := make(map[int]msgs.Order)
	acceptedOrders := make(map[int]bool)     // used as a set
	thisElevatorOrders := make(map[int]bool) // used as a set
	thisElevatorOrdersUpdated := false

	dbg_placeOrderCh := make(chan msgs.Debug_placeOrderMsg)
	dbg_acceptOrderCh := make(chan msgs.Debug_acceptOrderMsg)
	go bcast.Receiver(port, dbg_placeOrderCh, dbg_acceptOrderCh)

	const fsmMaxFloor int = 4
	fsmStatus := msgs.ElevatorStatus{ID: *id_ptr, Direction: msgs.Up, Floor: 1 + rnd.Intn(fsmMaxFloor-1)}

	fmt.Println("[fsm] started at: ", fsmStatus)
	thisElevatorHeartbeatCh <- msgs.Heartbeat{SenderID: *id_ptr, Status: fsmStatus, AcceptedOrders: []msgs.Order{}}

	var elevators []msgs.ElevatorStatus

	for {
		select {
		case elevators = <-elevatorsStatusCh:	// debugging
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
					fmt.Printf("[thisElevatorHeartbeatCh]: Warn: orderID %v didn't exist")
				}
			}

			thisElevatorHeartbeatCh <- msgs.Heartbeat{SenderID: *id_ptr, Status: fsmStatus, AcceptedOrders: acceptedOrderList}
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

		case safeMsg := <-safeOrderCh:	// Here
			fmt.Printf("[safeOrderCh]: %v\n", safeMsg)
			if safeMsg.RecieverID == *id_ptr {
				if _, exists := orders[safeMsg.Order.ID]; exists {
					acceptedOrders[safeMsg.Order.ID] = true

					scoreMap := make(map[string]int)
					for _, elevator := range elevators {
						scoreMap[elevator.ID] = calculateOrderScore(elevator, orders[safeMsg.Order.ID])
					}

					// find best (lowest) score
					bestID := *id_ptr
					for id, score := range scoreMap {
						if score < scoreMap[bestID] {
							bestID = id
						}
					}

					fmt.Printf("[orderHandler]: elevator %v should take order %v (%v)\n", bestID, safeMsg.Order.ID, scoreMap)
					if bestID != *id_ptr {
						takeOrderMsg := msgs.TakeOrderMsg{SenderID: *id_ptr, RecieverID: bestID, Order: orders[safeMsg.Order.ID]}
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
