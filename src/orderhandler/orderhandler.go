package orderhandler

import (
	"../elevio"
	"../fsm"
	"../msgs"
	"fmt"
	"../go-nonblockingchan"
	"sync"
	"time"
)

func createOrderID(floor int, button elevio.ButtonType, num_floors int) int {
	//elevIDint, _ := strconv.Atoi(elevID)
	return num_floors * int(button) + floor
}

func OrderHandler(thisID string,
	/* Read channels */
	elevatorStatusCh *nbc.NonBlockingChan,
	allElevatorsHeartbeatCh *nbc.NonBlockingChan,
	placedHallOrderCh *nbc.NonBlockingChan,
	safeOrderCh *nbc.NonBlockingChan,
	thisTakeOrderCh *nbc.NonBlockingChan,
	downedElevatorsCh *nbc.NonBlockingChan,
	completedOrderThisElevCh *nbc.NonBlockingChan,
	completedOrderOtherElevCh *nbc.NonBlockingChan,
	/* Write channels */
	addHallOrderCh *nbc.NonBlockingChan,
	broadcastTakeOrderCh *nbc.NonBlockingChan,
	placedOrderCh *nbc.NonBlockingChan,
	deleteHallOrderCh *nbc.NonBlockingChan,
	completedOrderCh *nbc.NonBlockingChan,
	thisElevatorHeartbeatCh *nbc.NonBlockingChan,
	turnOnLightsCh *nbc.NonBlockingChan,
	/* Sync */
	wg *sync.WaitGroup) {

	placedOrders := make(map[int]msgs.Order)     // all placed placedOrders at this elevator
	acceptedOrders := make(map[int]msgs.Order)   // set of accepted orderIDs
	takenOrders := make(map[int]msgs.Order)      // set of order this elevator will take
	elevators := make(map[string]msgs.Heartbeat) //storage of all heartbeats

	// Wait until all modules are initialized
	wg.Done()
	fmt.Println("[fsm]: initialized")
	wg.Wait()
	fmt.Println("[fsm]: starting")

	for {
		select {
		case msg, _ := <-thisTakeOrderCh.Recv:
			order := msg.(msgs.TakeOrderMsg)
			// TODO: Verify that only other elevators orders come in here. Or else lights will be wrong

			if order.SenderID == thisID {
				fmt.Printf("[orderhandler]: Warning: thisTakeOrder from self: %v\n", order)
				// TODO: Lights may be wrong
				addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Order.Floor, Button: order.Order.Type, LightOn: true}
			} else {
				addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Order.Floor, Button: order.Order.Type, LightOn: false}
			}
			takenOrders[order.Order.ID] = order.Order

		case msg, _ := <-safeOrderCh.Recv:
			order := msg.(msgs.SafeOrderMsg)

			if order.ReceiverID == thisID {
				if order, exists := placedOrders[order.Order.ID]; exists {
					acceptedOrders[order.ID] = order

					// calculate scores
					scoreMap := make(map[string]float64)
					for _, elevator := range elevators {
						scoreMap[elevator.SenderID] = fsm.EstimatedCompletionTime(elevator.Status, fsm.OrderEvent{Floor: order.Floor, Button: order.Type})
					}
					// find best (lowest) score
					bestID := thisID
					for i, score := range scoreMap {
						if score < scoreMap[bestID] {
							bestID = i
						}
					}
					// broadcast
					fmt.Printf("[orderHandler]: elevator %v should take order %v\n", bestID, order.ID)
					takeOrderMsg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: bestID, Order: order}
					//fmt.Println("[orderhandler]: writing to broadcast (best)")
					broadcastTakeOrderCh.Send <- takeOrderMsg
					//fmt.Println("[orderhandler]: broadcast (best) done")

					if bestID == thisID {
						takenOrders[order.ID] = order
						addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Floor,
							Button:  order.Type,
							LightOn: true}
					}
				} else {
					fmt.Println("[orderHandler]: safeOrderCh: order didn't exist")
					// TODO: error handling
				}
			}

		case msg, _ := <-downedElevatorsCh.Recv:
			//TODO: test that is safe to send empty lists here
			downedElevators := msg.([]msgs.Heartbeat)
			for _, lastHeartbeat := range downedElevators {
				// elevator is down
				fmt.Printf("[orderHandler]: Down: %+v %v\n", lastHeartbeat.SenderID, lastHeartbeat.AcceptedOrders)
				// Add taken orders
				for orderID, order := range lastHeartbeat.TakenOrders {
					takenOrders[orderID] = order
					//fmt.Println("[orderhandler]: writing to addHallOrder (take)")
					addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Floor, Button: order.Type, LightOn: false}
					//fmt.Println("[orderhandler]: addHallOrder (take) done")
				}
				// Add accepted orders
				for orderID, order := range lastHeartbeat.AcceptedOrders {
					acceptedOrders[orderID] = order
					//fmt.Println("[orderhandler]: writing to addHallOrder (acc)")
					addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Floor, Button: order.Type, LightOn: true}
					//fmt.Println("[orderhandler]: addHallOrder (acc) done")
				}
				delete(elevators, lastHeartbeat.SenderID) // Not sure about this ? can be handled by heartbeat channel
			}

		case msg, _ := <-placedHallOrderCh.Recv:
			buttonEvent := msg.(fsm.OrderEvent)

			orderID := createOrderID(buttonEvent.Floor, buttonEvent.Button, fsm.N_FLOORS)
			order := msgs.Order{ID: orderID, Floor: buttonEvent.Floor, Type: buttonEvent.Button}
			placedOrders[orderID] = order

			//fmt.Println("[orderhandler]: writing to placedOrder")
			placedOrderCh.Send <- order
			//fmt.Println("[orderhandler]: placedOrder done")

		case msg, _ := <-completedOrderOtherElevCh.Recv:
			completedOrder := msg.(msgs.Order)

			for _, order := range placedOrders {
				if order.Floor == completedOrder.Floor &&
					order.Type == completedOrder.Type {
					delete(placedOrders, order.ID)
				}
			}
			for _, order := range acceptedOrders {
				if order.Floor == completedOrder.Floor &&
					order.Type == completedOrder.Type {
					delete(acceptedOrders, order.ID)
				}
			}
			for _, order := range takenOrders {
				if order.Floor == completedOrder.Floor &&
					order.Type == completedOrder.Type {
					delete(takenOrders, order.ID)
				}
			}

			deleteHallOrderCh.Send <- fsm.OrderEvent{Floor: completedOrder.Floor, Button: completedOrder.Type}

		case msg, _ := <-completedOrderThisElevCh.Recv:
			completedOrders := msg.([]fsm.OrderEvent)

			// find and remove all equivalent placedOrders
			for _, completedOrder := range completedOrders {
				orderID := createOrderID(completedOrder.Floor, completedOrder.Button, fsm.N_FLOORS)
				fmt.Printf("[orderHandler]: completed order %v\n", orderID)
				// broadcast to network that order is completed
				//if order, exists := takenOrders[orderID]; exists {

				// TODO: the orderID calculation will only be correct for orders from this elevator
				// an order taken from another elevator may need its original orderID in order to be transmitted properly

				//} else {

				//	// TODO: this triggers for all cab calls
				if completedOrder.Button != elevio.BT_Cab {
					completedOrderCh.Send <- msgs.Order{ID: orderID, Floor: completedOrder.Floor, Type: completedOrder.Button}
					//fmt.Println("[orderHandler]: warn: completed non-taken order")
				}
				//}
				//delete order
				delete(takenOrders, orderID)
				delete(acceptedOrders, orderID)
				delete(placedOrders, orderID)
			}

		case msg, _ := <-elevatorStatusCh.Recv:
			elevatorStatus := msg.(fsm.Elevator)

			// make deep copy of accepted and taken orders
			acceptedOrdersDeepCopy := make(map[int]msgs.Order)
			for k, v := range acceptedOrders {
				acceptedOrdersDeepCopy[k] = v
			}

			takenOrdersDeepCopy := make(map[int]msgs.Order)
			for k, v := range takenOrders {
				takenOrdersDeepCopy[k] = v
			}

			// build heartbeat
			heartbeat := msgs.Heartbeat{SenderID: thisID,
				Status:         elevatorStatus,
				AcceptedOrders: acceptedOrdersDeepCopy,
				TakenOrders:    takenOrdersDeepCopy}

			thisElevatorHeartbeatCh.Send <- heartbeat

		case msg, _ := <-allElevatorsHeartbeatCh.Recv:
			allElevatorsHeartbeat := msg.([]msgs.Heartbeat)

			var turnOnLights [fsm.N_FLOORS][fsm.N_BUTTONS]bool
			// lights from other elevators
			for _, elevatorHeartbeat := range allElevatorsHeartbeat {
				elevators[elevatorHeartbeat.SenderID] = elevatorHeartbeat
				if elevatorHeartbeat.SenderID != thisID {
					for _, acceptedOrder := range elevatorHeartbeat.AcceptedOrders {
						turnOnLights[acceptedOrder.Floor][acceptedOrder.Type] = true
					}
				}
			}

			// lights from this elevator
			for _, acceptedOrder := range acceptedOrders {
				turnOnLights[acceptedOrder.Floor][acceptedOrder.Type] = true
			}

			if len(turnOnLights) > 0 {
				turnOnLightsCh.Send <- turnOnLights
			}

			// debug

			//var elevatorIDList []string
			//for id, _ := range elevators {
			//	elevatorIDList = append(elevatorIDList, id)
			//}
			//fmt.Printf("[orderHandler]: elevators: %v\n", elevatorIDList)

		case <-time.After(15 * time.Second):
			// an (empty) event every second, avoids some forms of locking
			var orderList []msgs.Order
			for orderID, _ := range acceptedOrders {
				if order, exists := placedOrders[orderID]; !exists {
					fmt.Println("[orderHandler]: ERROR: have accepted non-existing order", orderID)
					continue
				} else {
					orderList = append(orderList, order)
				}
			}

			fmt.Printf("[orderHandler]: acceptedList: %v\n", orderList)
		}
	}
}
