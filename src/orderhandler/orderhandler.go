package orderhandler

import (
	"../elevio"
	"../fsm"
	"../go-nonblockingchan"
	"../msgs"
	"log"
	"os"
	"sync"
	"time"
)

var Info *log.Logger

func createOrderID(floor int, button elevio.ButtonType, num_floors int) int {
	//elevIDint, _ := strconv.Atoi(elevID)
	return num_floors*int(button) + floor
}

func OrderHandler(thisID string,
	/* Read channels */
	elevatorStatusCh *nbc.NonBlockingChan,
	allElevatorsHeartbeatCh *nbc.NonBlockingChan,
	placedHallOrderCh *nbc.NonBlockingChan,
	safeOrderCh *nbc.NonBlockingChan,
	thisTakeOrderCh *nbc.NonBlockingChan,
	downedElevatorsCh *nbc.NonBlockingChan,
	completedHallOrdersThisElevCh *nbc.NonBlockingChan,
	completedHallOrderOtherElevCh *nbc.NonBlockingChan,
	/* Write channels */
	addHallOrderCh *nbc.NonBlockingChan,
	broadcastTakeOrderCh *nbc.NonBlockingChan,
	placedOrderCh *nbc.NonBlockingChan,
	deleteHallOrderCh *nbc.NonBlockingChan,
	completedOrderCh *nbc.NonBlockingChan,
	thisElevatorHeartbeatCh *nbc.NonBlockingChan,
	updateLightsCh *nbc.NonBlockingChan,
	/* Sync */
	wg *sync.WaitGroup) {

	Info = log.New(os.Stdout, "[orderhandler]: ", 0)

	placedOrders := make(map[int]msgs.Order)     // all placed placedOrders at this elevator
	acceptedOrders := make(map[int]msgs.Order)   // set of accepted orderIDs
	chosenElevatorForOrder := make(map[int]string)	// set of elevatorID for an acceptedOrder
	takenOrders := make(map[int]msgs.Order)      // set of order this elevator will take
	elevators := make(map[string]msgs.Heartbeat) //storage of all heartbeats

	// Wait until all modules are initialized
	wg.Done()
	Info.Println("initialized")
	wg.Wait()
	Info.Println("starting")

	for {
		select {
		case msg, _ := <-thisTakeOrderCh.Recv:
			order := msg.(msgs.TakeOrderMsg)

			if order.SenderID == thisID {
				Info.Printf("thisTakeOrder from self: %v\n", order)
				// TODO: Lights may be wrong
				addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Order.Floor, Button: order.Order.Type, TurnLightOn: true}
			} else {
				addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Order.Floor, Button: order.Order.Type, TurnLightOn: false}
			}
			takenOrders[order.Order.ID] = order.Order

		case msg, _ := <-safeOrderCh.Recv:
			order := msg.(msgs.SafeOrderMsg)

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
				chosenElevatorForOrder[order.ID] = bestID
				// broadcast
				Info.Printf("elevator %v should take order %v\n", bestID, order.ID)
				takeOrderMsg := msgs.TakeOrderMsg{SenderID: thisID, ReceiverID: bestID, Order: order}
				//Info.Println("writing to broadcast (best)")
				broadcastTakeOrderCh.Send <- takeOrderMsg
				//Info.Println("broadcast (best) done")

				if bestID == thisID {
					takenOrders[order.ID] = order
					addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Floor,
						Button:      order.Type,
						TurnLightOn: true}
				}
			} else {
				Info.Println("safeOrderCh: order didn't exist")
				// TODO: error handling
			}

		case msg, _ := <-downedElevatorsCh.Recv:
			//TODO: test that is safe to send empty lists here
			downedElevators := msg.([]msgs.Heartbeat)
			for _, lastHeartbeat := range downedElevators {
				// elevator is down
				Info.Printf("Down: %+v %v\n", lastHeartbeat.SenderID, lastHeartbeat.AcceptedOrders)
				// Add taken orders
				for orderID, order := range lastHeartbeat.TakenOrders {
					takenOrders[orderID] = order
					//Info.Println("writing to addHallOrder (take)")
					addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Floor, Button: order.Type, TurnLightOn: false}
					//Info.Println("addHallOrder (take) done")
				}
				// Add accepted orders
				for orderID, order := range lastHeartbeat.AcceptedOrders {
					acceptedOrders[orderID] = order
					chosenElevatorForOrder[orderID] = lastHeartbeat.ChosenElevatorForOrder[orderID]
					//Info.Println("writing to addHallOrder (acc)")
					addHallOrderCh.Send <- fsm.OrderEvent{Floor: order.Floor, Button: order.Type, TurnLightOn: true}
					//Info.Println("addHallOrder (acc) done")
				}
				delete(elevators, lastHeartbeat.SenderID) // Not sure about this ? can be handled by heartbeat channel
			}

		case msg, _ := <-placedHallOrderCh.Recv:
			buttonEvent := msg.(fsm.OrderEvent)

			orderID := createOrderID(buttonEvent.Floor, buttonEvent.Button, fsm.N_FLOORS)
			order := msgs.Order{ID: orderID, Floor: buttonEvent.Floor, Type: buttonEvent.Button}
			placedOrders[orderID] = order

			//Info.Println("writing to placedOrder")
			placedOrderCh.Send <- order
			//Info.Println("placedOrder done")

		case msg, _ := <-completedHallOrderOtherElevCh.Recv:
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
					delete(chosenElevatorForOrder, order.ID)
				}
			}
			for _, order := range takenOrders {
				if order.Floor == completedOrder.Floor &&
					order.Type == completedOrder.Type {
					delete(takenOrders, order.ID)
				}
			}

			deleteHallOrderCh.Send <- fsm.OrderEvent{Floor: completedOrder.Floor, Button: completedOrder.Type}

		case msg, _ := <-completedHallOrdersThisElevCh.Recv:
			completedOrders := msg.([]fsm.OrderEvent)

			// find and remove all equivalent placedOrders
			for _, completedOrder := range completedOrders {
				orderID := createOrderID(completedOrder.Floor, completedOrder.Button, fsm.N_FLOORS)
				Info.Printf("completed order %v\n", orderID)
				// broadcast to network that order is completed
				//if order, exists := takenOrders[orderID]; exists {

				// TODO: the orderID calculation will only be correct for orders from this elevator
				// an order taken from another elevator may need its original orderID in order to be transmitted properly

				//} else {

				if completedOrder.Button != elevio.BT_Cab {
					completedOrderCh.Send <- msgs.Order{ID: orderID, Floor: completedOrder.Floor, Type: completedOrder.Button}
					//Info.Println("warn: completed non-taken order")
				}
				//}
				//delete order
				delete(takenOrders, orderID)
				delete(acceptedOrders, orderID)
				delete(chosenElevatorForOrder, orderID)
				delete(placedOrders, orderID)
			}

		case msg, _ := <-elevatorStatusCh.Recv:
			elevatorStatus := msg.(fsm.Elevator)

			// make deep copy of accepted and taken orders
			acceptedOrdersDeepCopy := make(map[int]msgs.Order)
			for k, v := range acceptedOrders {
				acceptedOrdersDeepCopy[k] = v
			}
			// make deep copy of chosenElevatorForOrder
			chosenElevatorForOrderDeepCopy := make(map[int]string)
			for k, v := range chosenElevatorForOrder {
				chosenElevatorForOrderDeepCopy[k] = v
			}

			takenOrdersDeepCopy := make(map[int]msgs.Order)
			for k, v := range takenOrders {
				takenOrdersDeepCopy[k] = v
			}

			// build heartbeat
			heartbeat := msgs.Heartbeat{SenderID: thisID,
				Status:         elevatorStatus,
				AcceptedOrders: acceptedOrdersDeepCopy,
				ChosenElevatorForOrder: chosenElevatorForOrderDeepCopy,
				TakenOrders:    takenOrdersDeepCopy}

			thisElevatorHeartbeatCh.Send <- heartbeat

		case msg, _ := <-allElevatorsHeartbeatCh.Recv:
			allElevatorsHeartbeat := msg.([]msgs.Heartbeat)

			var updateLights [fsm.N_FLOORS][fsm.N_BUTTONS]bool

			// update elevators
			for _, elevatorHeartbeat := range allElevatorsHeartbeat {
				elevators[elevatorHeartbeat.SenderID] = elevatorHeartbeat
			}
			// update lights: Using elevators or allElevatorsHeartbeat ???
			for _, elevatorHeartbeat := range allElevatorsHeartbeat {
				for _, acceptedOrder := range elevatorHeartbeat.AcceptedOrders {
					chosenElevatorID := elevatorHeartbeat.ChosenElevatorForOrder[acceptedOrder.ID]
					// find heartbeat for chosenElevatorID in allElevatorsHeartbeat
					elevatorWithIDFound := false
					for _, chosenElevatorHeartbeat := range allElevatorsHeartbeat {
						if chosenElevatorHeartbeat.SenderID == chosenElevatorID {
							elevatorWithIDFound = true
							chosenElevatorTakenOrder := chosenElevatorHeratbeat.TakenOrders
							chosenElevatorStatus := chosenElevatorHeratbeat.Status
							if _, exists := chosenElevatorTakenOrder[acceptedOrder.ID], exists{
								if chosenElevatorStatus.Orders[acceptedOrder.Floor][acceptedOrder.Button]{
									fmt.Println("[order]: Order is taken and registered in fsm. OK")
									fmt.Println("         Order: \v", acceptedOrder)
									fmt.Println("         Master: \v", elevatorHeartbeat.SenderID)
									fmt.Println("         Slave: \v", chosenElevatorID)
									updateLights[acceptedOrder.Floor][acceptedOrder.Button] = true
									//break
								} else { //debugging
									fmt.Println("[order]: Order is taken, but not registered in fsm. SERIOUS PROBLEM")
									fmt.Println("         Order: \v", acceptedOrder)
									fmt.Println("         Master: \v", elevatorHeartbeat.SenderID)
									fmt.Println("         Slave: \v", chosenElevatorID)
								}
							} else {
								if chosenElevatorStatus.Orders[acceptedOrder.Floor][acceptedOrder.Button]{
									fmt.Println("[order]: Order is not taken, but registered in fsm, but accepted. ERROR")
									fmt.Println("         Order: \v", acceptedOrder)
									fmt.Println("         Master: \v", elevatorHeartbeat.SenderID)
									fmt.Println("         Slave: \v", chosenElevatorID)
								} else {
									fmt.Println("[order]: Order is not taken and is not registered in fsm, but accepted. SERIOUS PROBLEM")
									fmt.Println("         Order: \v", acceptedOrder)
									fmt.Println("         Master: \v", elevatorHeartbeat.SenderID)
									fmt.Println("         Slave: \v", chosenElevatorID)
									// The order can have been completed and any of the CompletedOrderMsg didnt arrive
									//
								}
							}
						}
					}
					if !elevatorWithIDFound {
						fmt.Println("[order]: Order is taken, but not registered in fsm. SERIOUS PROBLEM")
						fmt.Println("         Order: \v", acceptedOrder)
						fmt.Println("         Master: \v", elevatorHeartbeat.SenderID)
						fmt.Println("         Slave: \v", chosenElevatorID)
					}
				}
			}
			// lights from this elevator
			for _, acceptedOrder := range acceptedOrders {
				updateLights[acceptedOrder.Floor][acceptedOrder.Type] = true
			}

			updateLightsCh.Send <- updateLights

		case <-time.After(15 * time.Second):
			// an (empty) event every second, avoids some forms of locking
			var orderList []msgs.Order
			for orderID, _ := range acceptedOrders {
				if order, exists := placedOrders[orderID]; !exists {
					Info.Printf("ERROR: have accepted non-existing order %v\n", orderID)
					continue
				} else {
					orderList = append(orderList, order)
				}
			}

			Info.Printf("acceptedList: %v\n", orderList)
		}
	}
}
