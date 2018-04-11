package network

import (
	"../comm/bcast"
	"../comm/peers"
	"../fsm"
	"../msgs"
	"fmt"
	"math/rand"
	"time"
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

//const server_ip = "129.241.187.38"
const port = 20010
const timeout = 1 * time.Second
const giveupAckwaitTimeout = 5 * time.Second
const N_FLOORS = 4 //import
const N_BUTTONS = 3

func Launch(id string,
	thisElevatorStatusCh <-chan msgs.Heartbeat, allElevatorsHeartbeatCh chan<- []msgs.Heartbeat, downedElevatorsCh chan<- []msgs.Heartbeat,
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
				fmt.Printf("[placedOrderRecvCh]: Warning, order id %v already exists, new order ignored\n", msg.Order.ID)
				break
			}
			ordersRecieved[msg.Order.ID] = msg.Order

			if msg.SenderID != id { // ignore internal msgs
				// Order transmitted from other node

				// acknowledge order
				ack := msgs.PlacedOrderAck{SenderID: id,
					ReceiverID: msg.SenderID,
					Order:      msg.Order}
				fmt.Printf("[placedOrderRecvCh]: Sending ack to %v for order %v\n", ack.ReceiverID, ack.Order.ID)
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
			if msg.ReceiverID == id { // ignore msgs to other nodes
				// Acknowledgement recieved from other node
				if _, ok := placeUnackedOrders[msg.Order.ID]; !ok {
					break // Not waiting for acknowledgment
				}

				fmt.Printf("[placedOrderAckRecvCh]: %v acknowledged\n", msg.Order.ID)
				delete(placeUnackedOrders, msg.Order.ID)

				// Order is safe since multiple elevators knows about it
				safeMsg := msgs.SafeOrderMsg{SenderID: id, ReceiverID: id, Order: msg.Order}
				safeOrderCh <- safeMsg
			}
		case msg := <-otherTakeOrderCh:
			takeOrderSendCh <- msg
			takeUnackedOrders[msg.Order.ID] = time.Now()
		case msg := <-takeOrderRecvCh:
			if msg.ReceiverID == id {
				thisTakeOrderCh <- msg

				ack := msgs.TakeOrderAck{SenderID: id, ReceiverID: msg.SenderID, Order: msg.Order}
				takeOrderAckSendCh <- ack
			}
		case msg := <-takeOrderAckRecvCh:
			if msg.ReceiverID == id {
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
				fmt.Println("[peerUpdateCh]: New: ", peerUpdate.New)
			}

			allElevatorsHeartbeatCh <- peerUpdate.Peers
		case order := <-completedOrderCh:
			fmt.Println("[orderCompletedCh]: ", order)

			completeOrderSendCh <- msgs.CompleteOrderMsg{Order: order}
		case msg := <-completeOrderRecvCh:
			if _, exists := ongoingOrders[msg.Order.ID]; exists {
				fmt.Println("[orderCompletedRecvCh]: ", msg.Order)
				delete(ongoingOrders, msg.Order.ID)
			}
		case partialHeartbeat := <-thisElevatorStatusCh:
			// heartbeat lacks id
			partialHeartbeat.SenderID = id

			peerStatusSendCh <- partialHeartbeat
			//var acceptedOrders []msgs.Order
			//for orderID, _ := range ongoingOrders {
			//	acceptedOrders = append(acceptedOrders, ordersRecieved[orderID])
			//}
			//heartbeat := msgs.Heartbeat{SenderID: id, Status: status, AcceptedOrders: acceptedOrders}
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
				msg := msgs.TakeOrderMsg{SenderID: id, ReceiverID: id,
					Order: ordersRecieved[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg

				delete(takeUnackedOrders, orderID)
			}
		}

		for orderID, t := range ongoingOrders {
			if time.Now().Sub(t) > 30*time.Second {
				fmt.Printf("[timeout]: complete not recieved for %v\n\t%v\n", orderID, ongoingOrders)

				msg := msgs.TakeOrderMsg{SenderID: id, ReceiverID: id,
					Order: ordersRecieved[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg
				delete(ongoingOrders, orderID)
				delete(ordersRecieved, orderID)
			}
		}
	}
}

// pseudo-orderHandler and fsm
func PseudoOrderHandlerAndFsm(id string, simAddr string, thisElevatorHeartbeatCh chan<- msgs.Heartbeat,
	allElevatorsHeartbeatCh <-chan []msgs.Heartbeat, downedElevatorsCh <-chan []msgs.Heartbeat,
	placedOrderCh chan<- msgs.Order, thisTakeOrderCh <-chan msgs.TakeOrderMsg, otherTakeOrderCh chan<- msgs.TakeOrderMsg,
	safeOrderCh <-chan msgs.SafeOrderMsg, completedOrderCh chan<- msgs.Order){//,turnOnLightsCh chan<- [N_FLOORS][N_BUTTONS]bool) {

	addHallOrderCh := make(chan fsm.OrderEvent)
	deleteHallOrderCh := make(chan fsm.OrderEvent)
	placedHallOrderCh := make(chan fsm.OrderEvent)
	completedHallOrderCh := make(chan []fsm.OrderEvent)
	elevatorStatusCh := make(chan fsm.Elevator)
	turnOnLightsCh := make(chan [N_FLOORS][N_BUTTONS]bool)
	go fsm.FSM(simAddr, addHallOrderCh, deleteHallOrderCh,
		placedHallOrderCh, completedHallOrderCh,
		elevatorStatusCh, turnOnLightsCh)
	var elevatorStatus fsm.Elevator

	orders := make(map[int]msgs.Order)
	acceptedOrders := make(map[int]bool)     // set of accepted orderIDs
	thisElevatorOrders := make(map[int]bool) // set of order this elevator will take

	thisElevatorOrdersUpdated := false
	fmt.Println("[fsm] started at: ", elevatorStatus)
	//thisElevatorHeartbeatCh <- msgs.Heartbeat{SenderID: id, Status: fsmStatus, AcceptedOrders: []msgs.Order{}}

	var elevators []msgs.Heartbeat

	for {
		select {
		case elevatorStatus = <-elevatorStatusCh: // Here
			var acceptedOrderList []msgs.Order
			for orderID, _ := range acceptedOrders {
				if order, exists := orders[orderID]; exists {
					acceptedOrderList = append(acceptedOrderList, order)
				} else {
					fmt.Printf("[thisElevatorHeartbeatCh]: Warn: orderID %v didn't exist")
				}
			}
			thisElevatorHeartbeatCh <- msgs.Heartbeat{SenderID: id,
				Status:         elevatorStatus,
				AcceptedOrders: acceptedOrderList}

		case allElevatorsHeartbeat := <-allElevatorsHeartbeatCh: // debugging. OK
			fmt.Printf("[orderHandler]: number of elevators: %v\n", len(elevators))

			var turnOnLights [N_FLOORS][N_BUTTONS]bool
			for _, elevatorHeartbeat := range allElevatorsHeartbeat {
				if elevatorHeartbeat.SenderID != id {
					for _, acceptedOrder := range elevatorHeartbeat.AcceptedOrders {
						turnOnLights[acceptedOrder.Floor][acceptedOrder.Type] = true
					}
				} // if else : Check if heartbeat of this elevator corresponds to the actual status
			}
			turnOnLightsCh <- turnOnLights // can be all false


		case downedElevators := <-downedElevatorsCh: // OK
			for _, lastHeartbeat := range downedElevators {
				// elevator is down
				fmt.Printf("[orderHandler]: down: %+v %v\n", lastHeartbeat.SenderID, lastHeartbeat.AcceptedOrders)
				// take order this elevator had
				for _, order := range lastHeartbeat.AcceptedOrders {
					orders[order.ID] = order
					addHallOrderCh <- fsm.OrderEvent{order.Floor, order.Type, false} //turn on/off lights? ???
				}
			}
		case <-time.After(20 * time.Second): // debugging. OK
			fmt.Println("[fsm] status: ", elevatorStatus)
		case orderEventSlice := <-completedHallOrderCh: // OK
			for _, completedOrder := range orderEventSlice {
				for orderID, _ := range thisElevatorOrders {
					if orders[orderID].Floor == completedOrder.Floor &&
						orders[orderID].Type == completedOrder.Button {
						fmt.Printf("[fsm->network]: completed order %v\n", orderID)
						// broadcast to network that order is completed
						completedOrderCh <- orders[orderID]
						// remove order from orderHandler/fsm
						thisElevatorOrdersUpdated = true // for debugging
						delete(acceptedOrders, orderID)
						delete(orders, orderID) // remove order from system
					}
				}
			}
		case msg := <-thisTakeOrderCh: // OK
			if _, exists := orders[msg.Order.ID]; !exists {
				fmt.Printf("[thisTakeOrderCh]: didnt have order %v,from before, %v\n", msg.Order.ID, orders)
				orders[msg.Order.ID] = msg.Order
				addHallOrderCh <- fsm.OrderEvent{msg.Order.Floor, msg.Order.Type, true} // turn on/off light? ???
			}
			// error checking
			if orders[msg.Order.ID] != msg.Order {
				fmt.Printf("[thisTakeOrderCh]: had different order with same ID \n\t(my)%+v\n\t(recv)%+v\n", orders[msg.Order.ID], msg.Order)
			}
			//acceptedOrders[msg.Order.ID] = true
			thisElevatorOrders[msg.Order.ID] = true
			thisElevatorOrdersUpdated = true // for debugging

		case buttonEvent := <-placedHallOrderCh: // OK

			// Create order with unique ID (atleast unique to this elevator)
			orderID := 0
			exists := true
			for exists {
				orderID = rnd.Intn(100000)
				_, exists = orders[orderID]
			}

			order := msgs.Order{ID: orderID, Floor: buttonEvent.Floor, Type: buttonEvent.Button}
			orders[orderID] = order
			placedOrderCh <- order
		case safeMsg := <-safeOrderCh:
			fmt.Printf("[safeOrderCh]: %v\n", safeMsg)
			if safeMsg.ReceiverID == id {
				if _, exists := orders[safeMsg.Order.ID]; exists {
					acceptedOrders[safeMsg.Order.ID] = true

					scoreMap := make(map[string]float64)
					for _, elevator := range elevators {
						scoreMap[elevator.SenderID] = fsm.EstimatedCompletionTime(elevator.Status, fsm.OrderEvent{Floor: safeMsg.Order.Floor, Button: safeMsg.Order.Type})
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
						takeOrderMsg := msgs.TakeOrderMsg{SenderID: id, ReceiverID: bestID, Order: orders[safeMsg.Order.ID]}
						otherTakeOrderCh <- takeOrderMsg
					} else {
						thisElevatorOrders[safeMsg.Order.ID] = true
						addHallOrderCh <- fsm.OrderEvent{safeMsg.Order.Floor, safeMsg.Order.Type, true} // turn on/off light? ???
						thisElevatorOrdersUpdated = true                                                // for debugging
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
