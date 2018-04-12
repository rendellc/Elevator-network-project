package network

import (
	"../comm/bcast"
	"../comm/peers"
	"../fsm"
	"../msgs"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

//const server_ip = "129.241.187.38"
const commonPort = 20010
const timeout = 1 * time.Second
const giveupAckwaitTimeout = 5 * time.Second
const N_FLOORS = 4 //import
const N_BUTTONS = 3

func Launch(thisID string,
	thisElevatorHeartbeatCh <-chan msgs.Heartbeat, allElevatorsHeartbeatCh chan<- []msgs.Heartbeat, downedElevatorsCh chan<- []msgs.Heartbeat,
	placedOrderCh <-chan msgs.Order, thisTakeOrderCh chan<- msgs.TakeOrderMsg, broadcastTakeOrderCh <-chan msgs.TakeOrderMsg,
	safeOrderCh chan<- msgs.SafeOrderMsg, completedOrderCh <-chan msgs.Order, wg *sync.WaitGroup) {

	placedOrderSendCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckSendCh := make(chan msgs.PlacedOrderAck)
	takeOrderAckSendCh := make(chan msgs.TakeOrderAck)
	takeOrderSendCh := make(chan msgs.TakeOrderMsg)
	completeOrderSendCh := make(chan msgs.CompleteOrderMsg)
	go bcast.Transmitter(commonPort, placedOrderSendCh, placedOrderAckSendCh, takeOrderAckSendCh, takeOrderSendCh, completeOrderSendCh)

	placedOrderRecvCh := make(chan msgs.PlacedOrderMsg)
	placedOrderAckRecvCh := make(chan msgs.PlacedOrderAck)
	takeOrderAckRecvCh := make(chan msgs.TakeOrderAck)
	takeOrderRecvCh := make(chan msgs.TakeOrderMsg)
	completeOrderRecvCh := make(chan msgs.CompleteOrderMsg)
	go bcast.Receiver(commonPort, placedOrderRecvCh, placedOrderAckRecvCh, takeOrderAckRecvCh, takeOrderRecvCh, completeOrderRecvCh)

	peerTxEnable := make(chan bool)
	peerStatusSendCh := make(chan msgs.Heartbeat)
	go peers.Transmitter(commonPort, peerTxEnable, peerStatusSendCh)

	peerUpdateCh := make(chan peers.PeerUpdate, 1)
	go peers.Receiver(commonPort, peerUpdateCh)

	// bookkeeping variables
	ordersRecieved := make(map[int]msgs.Order)    // list of all placed orders to/from all elevators
	placeUnackedOrders := make(map[int]time.Time) // time is time when added
	takeUnackedOrders := make(map[int]time.Time)  // time is time when added
	ongoingOrders := make(map[int]time.Time)      // time is time when added

	// Wait until all modules are initialized
	wg.Done()
	fmt.Println("[Network]: initialized")
	wg.Wait()
	fmt.Println("[Network]: starting")
	for {
		select {
		case msg := <-placedOrderRecvCh:
			// store order
			if _, exists := ordersRecieved[msg.Order.ID]; exists {
				fmt.Printf("[placedOrderRecvCh]: Warning, order id %v already exists, new order ignored\n", msg.Order.ID)
				break
			}
			ordersRecieved[msg.Order.ID] = msg.Order

			if msg.SenderID != thisID { // ignore internal msgs
				// Order transmitted from other node

				// acknowledge order
				ack := msgs.PlacedOrderAck{SenderID: thisID,
					ReceiverID: msg.SenderID,
					Order:      msg.Order}
				fmt.Printf("[placedOrderRecvCh]: Sending ack to %v for order %v\n", ack.ReceiverID, ack.Order.ID)
				placedOrderAckSendCh <- ack
			}
		case order := <-placedOrderCh:
			// This node has sent out an order. Needs to listen for acks
			if ackwait, exists := placeUnackedOrders[order.ID]; exists {
				fmt.Printf("[placedOrderRecvCh]: Warning, ack wait id %v already exists %v\n", order.ID, time.Now().Sub(ackwait))
			}
			placeUnackedOrders[order.ID] = time.Now()

			placedOrderSendCh <- msgs.PlacedOrderMsg{SenderID: thisID, Order: order}

		case msg := <-placedOrderAckRecvCh:
			if msg.ReceiverID == thisID { // ignore msgs to other nodes
				// Acknowledgement recieved from other node
				if _, exists := placeUnackedOrders[msg.Order.ID]; !exists {
					//fmt.Printf("[placedOrderAckRecvCh]: not waiting on ack %+v\n", msg.Order)
					break // Not waiting for acknowledgment
					// TODO: maybe count how often we end up here?
				}

				fmt.Printf("[placedOrderAckRecvCh]: %v acknowledged\n", msg.Order.ID)
				delete(placeUnackedOrders, msg.Order.ID)

				// Order is safe since multiple elevators knows about it, notify orderHandler
				// TODO: RecieverID shouldn't be filled in?
				safeMsg := msgs.SafeOrderMsg{SenderID: thisID, ReceiverID: thisID, Order: msg.Order}
				safeOrderCh <- safeMsg
			}
		case msg := <-broadcastTakeOrderCh:
			takeOrderSendCh <- msg
			takeUnackedOrders[msg.Order.ID] = time.Now()
		case msg := <-takeOrderRecvCh:
			if msg.ReceiverID == thisID {
				thisTakeOrderCh <- msg

				ack := msgs.TakeOrderAck{SenderID: thisID, ReceiverID: msg.SenderID, Order: msg.Order}
				takeOrderAckSendCh <- ack
			}
		case msg := <-takeOrderAckRecvCh:
			if msg.ReceiverID == thisID {
				fmt.Printf("[takeOrderAckRecvCh]: Recieved ack: %v\n", msg)
				delete(takeUnackedOrders, msg.Order.ID)

			}

			// contains all ongoing orders from all elevators
			ongoingOrders[msg.Order.ID] = time.Now()
		case peerUpdate := <-peerUpdateCh:
			if len(peerUpdate.Lost) > 0 {

				var downedElevators []msgs.Heartbeat
				for _, lastHeartbeat := range peerUpdate.Lost {
					fmt.Printf("[peerUpdateCh]: lost %v\n", lastHeartbeat.SenderID)
					downedElevators = append(downedElevators, lastHeartbeat)
				}

				downedElevatorsCh <- downedElevators
			}

			if len(peerUpdate.New) > 0 {
				fmt.Println("[peerUpdateCh]: New: ", peerUpdate.New)
			}

			// TODO: this sometimes deadlocks with thisElevatorHeartbeatCh, probably due to some circular dependecy
			//fmt.Println("[network]: writing to allElevatorsHeartbeatCh")
			allElevatorsHeartbeatCh <- peerUpdate.Peers
			//fmt.Println("[network]: allElevatorsHeartbeatCh done")
		case order := <-completedOrderCh:
			fmt.Println("[orderCompletedCh]: ", order)

			completeOrderSendCh <- msgs.CompleteOrderMsg{Order: order}
		case msg := <-completeOrderRecvCh:
			if _, exists := ongoingOrders[msg.Order.ID]; exists {
				fmt.Println("[orderCompletedRecvCh]: ", msg.Order)
				delete(ongoingOrders, msg.Order.ID)
			}
		case heartbeat := <-thisElevatorHeartbeatCh:
			// heartbeat lacks thisID
			heartbeat.SenderID = thisID

			peerStatusSendCh <- heartbeat
			//var acceptedOrders []msgs.Order
			//for orderID, _ := range ongoingOrders {
			//	acceptedOrders = append(acceptedOrders, ordersRecieved[orderID])
			//}
			//heartbeat := msgs.Heartbeat{SenderID: thisID, Status: status, AcceptedOrders: acceptedOrders}
		case <-time.After(1 * time.Second):
			// an (empty) event every second, avoids some forms of locking
			//fmt.Println("[network]: running")
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
				msg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: thisID,
					Order: ordersRecieved[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg

				delete(takeUnackedOrders, orderID)
			}
		}

		for orderID, t := range ongoingOrders {
			if time.Now().Sub(t) > 30*time.Second {
				fmt.Printf("[timeout]: complete not recieved for %v\n\t%v\n", orderID, ongoingOrders)

				msg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: thisID,
					Order: ordersRecieved[orderID]} // TODO: get information to fill out order floor etc. elevator behaviour shouldn't need this
				thisTakeOrderCh <- msg
				delete(ongoingOrders, orderID)
				delete(ordersRecieved, orderID)
			}
		}
	}
}

// pseudo-orderHandler and fsm
func PseudoOrderHandlerAndFsm(thisID string, simAddr string, thisElevatorHeartbeatCh chan<- msgs.Heartbeat,
	allElevatorsHeartbeatCh <-chan []msgs.Heartbeat, downedElevatorsCh <-chan []msgs.Heartbeat,
	placedOrderCh chan<- msgs.Order, thisTakeOrderCh <-chan msgs.TakeOrderMsg, broadcastTakeOrderCh chan<- msgs.TakeOrderMsg,
	safeOrderCh <-chan msgs.SafeOrderMsg, completedOrderCh chan<- msgs.Order, wg *sync.WaitGroup) { //,turnOnLightsCh chan<- [N_FLOORS][N_BUTTONS]bool) {

	addHallOrderCh := make(chan fsm.OrderEvent)
	deleteHallOrderCh := make(chan fsm.OrderEvent)
	placedHallOrderCh := make(chan fsm.OrderEvent)
	completedHallOrderCh := make(chan []fsm.OrderEvent)
	elevatorStatusCh := make(chan fsm.Elevator)
	turnOnLightsCh := make(chan [N_FLOORS][N_BUTTONS]bool)

	go fsm.FSM(simAddr, addHallOrderCh, deleteHallOrderCh,
		placedHallOrderCh, completedHallOrderCh,
		elevatorStatusCh, turnOnLightsCh, wg)

	var elevatorStatus fsm.Elevator

	orders := make(map[int]msgs.Order)
	acceptedOrders := make(map[int]bool)     // set of accepted orderIDs
	thisElevatorOrders := make(map[int]bool) // set of order this elevator will take

	thisElevatorOrdersUpdated := false

	// Bookkeeping
	elevators := make(map[string]msgs.Heartbeat)

	// Wait until all modules are initialized
	wg.Done()
	fmt.Println("[PseudoOrderHandler]: initialized")
	wg.Wait()
	fmt.Println("[PseudoOrderHandler]: starting")
	for {
		select {
		case elevatorStatus = <-elevatorStatusCh: // Here
			var acceptedOrderList []msgs.Order
			for orderID, _ := range acceptedOrders {
				if order, exists := orders[orderID]; exists {
					acceptedOrderList = append(acceptedOrderList, order)
				} else {
					fmt.Printf("[elevatorStatusCh]: Warn: orderID %v dthisIDn't exist")
				}
			}

			heartbeat := msgs.Heartbeat{SenderID: thisID,
				Status:         elevatorStatus,
				AcceptedOrders: acceptedOrderList}

			//TODO: deadlock zone, be careful!
			fmt.Println("[network]: writing to thisElevatorHeartbeatCh")
			thisElevatorHeartbeatCh <- heartbeat
			fmt.Println("[network]: thisElevatorHeartbeatCh done")

		case allElevatorsHeartbeat := <-allElevatorsHeartbeatCh: // debugging. OK

			var turnOnLights [N_FLOORS][N_BUTTONS]bool
			for _, elevatorHeartbeat := range allElevatorsHeartbeat {
				//fmt.Printf("[peers]: Peer: %+v\n", elevatorHeartbeat)
				elevators[elevatorHeartbeat.SenderID] = elevatorHeartbeat

				for _, acceptedOrder := range elevatorHeartbeat.AcceptedOrders {
					turnOnLights[acceptedOrder.Floor][acceptedOrder.Type] = true
				}
			}

			// debug
			var elevatorIDList []string
			for id, _ := range elevators {
				elevatorIDList = append(elevatorIDList, id)
			}

			fmt.Printf("[orderHandler]: elevators: %v\n", elevatorIDList)
			turnOnLightsCh <- turnOnLights
		case downedElevators := <-downedElevatorsCh: // OK
			for _, lastHeartbeat := range downedElevators {
				// elevator is down
				fmt.Printf("[orderHandler]: down: %+v %v\n", lastHeartbeat.SenderID, lastHeartbeat.AcceptedOrders)
				// take order this elevator had
				for _, order := range lastHeartbeat.AcceptedOrders {
					orders[order.ID] = order
					addHallOrderCh <- fsm.OrderEvent{order.Floor, order.Type, false} // quietly take order without turning on lights
				}

				delete(elevators, lastHeartbeat.SenderID)
			}
		case completedOrders := <-completedHallOrderCh: // OK
			for _, completedOrder := range completedOrders {
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
				addHallOrderCh <- fsm.OrderEvent{msg.Order.Floor, msg.Order.Type, true}
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
			if safeMsg.ReceiverID == thisID {
				if _, exists := orders[safeMsg.Order.ID]; exists {
					acceptedOrders[safeMsg.Order.ID] = true

					scoreMap := make(map[string]float64)
					scoreMap[thisID] = -1
					for _, elevator := range elevators {
						scoreMap[elevator.SenderID] = fsm.EstimatedCompletionTime(elevator.Status, fsm.OrderEvent{Floor: safeMsg.Order.Floor, Button: safeMsg.Order.Type})
					}

					//fmt.Printf("[orderHandler]: scoreMap: %+v\n", scoreMap)
					//fmt.Printf("[orderHandler]: All Statuses: %v\n", elevators)

					// find best (lowest) score
					bestID := thisID
					for i, score := range scoreMap {
						if score < scoreMap[bestID] {
							bestID = i
						}
					}

					fmt.Printf("[orderHandler]: elevator %v should take order %v\n", bestID, safeMsg.Order.ID)
					takeOrderMsg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: bestID, Order: orders[safeMsg.Order.ID]}
					broadcastTakeOrderCh <- takeOrderMsg

					if bestID == thisID {
						thisElevatorOrders[safeMsg.Order.ID] = true
						addHallOrderCh <- fsm.OrderEvent{Floor: safeMsg.Order.Floor,
							Button:  safeMsg.Order.Type,
							LightOn: true}
						thisElevatorOrdersUpdated = true // for debugging
					}
				} else {
					fmt.Println("[orderHandler]: order didn't exist")
				}
			}
		case <-time.After(1 * time.Second):
			// an (empty) event every second, avoids some forms of locking
			//fmt.Println("[orderHandler]: running")
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
