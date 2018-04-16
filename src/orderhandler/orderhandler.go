package orderhandler

import (
	"../elevio"
	"../fsm"
	"../go-nonblockingchan"
	"../msgs"
	"log"
	"os"
	"sync"
)

var Info *log.Logger

func createOrderID(floor int, button elevio.ButtonType, num_floors int) int {
	return num_floors*int(button) + floor
}

func OrderHandler(thisID string,
	/* Read channels */
	placedHallOrder_fsmCh *nbc.NonBlockingChan,
	redundantOrder_commhandlerCh *nbc.NonBlockingChan,
	takeOrder_commhandlerCh *nbc.NonBlockingChan,
	completedHallOrdersThisElev_fsmCh *nbc.NonBlockingChan,
	completedHallOrderOtherElevCh *nbc.NonBlockingChan,
	downedElevators_commhandlerCh *nbc.NonBlockingChan,
	elevatorStatus_fsmCh *nbc.NonBlockingChan,
	allElevatorsHeartbeat_commhandlerCh *nbc.NonBlockingChan,
	/* Write channels */
	placedOrder_commhandlerCh *nbc.NonBlockingChan,
	assignOrder_commhandlerCh *nbc.NonBlockingChan,
	addHallOrder_fsmCh *nbc.NonBlockingChan,
	completedOrder_commhandlerCh *nbc.NonBlockingChan,
	deleteHallOrder_fsmCh *nbc.NonBlockingChan,
	thisElevatorHeartbeat_commhandlerCh *nbc.NonBlockingChan,
	updateLights_fsmCh *nbc.NonBlockingChan,
	/* Sync */
	wg *sync.WaitGroup) {

	placedOrders := make(map[int]msgs.Order)       // placed orders at this elevator
	acceptedOrders := make(map[int]msgs.Order)     // accepted orders of this elevator (master)
	chosenElevatorForOrder := make(map[int]string) // chosen elevator (slave) to complete an accepted order
	assignedOrders := make(map[int]msgs.Order)     // assigned orders to this elevator (slave)
	elevators := make(map[string]msgs.Heartbeat)   // storage of the last received elevator heartbeats

	Info = log.New(os.Stdout, "[orderhandler]: ", 0)
	// Wait until all modules are initialized
	wg.Done()
	Info.Println("initialized")
	wg.Wait()
	Info.Println("starting")

	for {
		select {

		case msg, _ := <-placedHallOrder_fsmCh.Recv:
			buttonEvent := msg.(fsm.OrderEvent)

			orderID := createOrderID(buttonEvent.Floor, buttonEvent.Button, fsm.N_FLOORS)
			order := msgs.Order{ID: orderID, Floor: buttonEvent.Floor, Type: buttonEvent.Button}
			placedOrders[orderID] = order
			placedOrder_commhandlerCh.Send <- order

		case msg, _ := <-redundantOrder_commhandlerCh.Recv:
			orderMsg := msg.(msgs.RedundantOrderMsg)

			if order, exists := placedOrders[orderMsg.Order.ID]; exists {
				acceptedOrders[order.ID] = order

				// calculate scores
				scoreMap := make(map[string]float64)
				for _, elevator := range elevators {
					scoreMap[elevator.SenderID] = fsm.EstimatedCompletionTime(elevator.Status,
						fsm.OrderEvent{Floor: order.Floor, Button: order.Type})
				}

				// find elevator ID with lowest score
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
				assignOrder_commhandlerCh.Send <- takeOrderMsg

				if bestID == thisID {
					assignedOrders[order.ID] = order
					addHallOrder_fsmCh.Send <- fsm.OrderEvent{Floor: order.Floor,
						Button: order.Type, TurnLightOn: true}
				}
			} else {
				Info.Print("redundant order %v didn't exist\n", orderMsg.Order.ID)
			}

		case msg, _ := <-takeOrder_commhandlerCh.Recv:
			order := msg.(msgs.TakeOrderMsg)

			if order.SenderID == thisID {
				Info.Printf("takeOrder_commhandlerCh: assigned order to itself: %v\n", order)
				addHallOrder_fsmCh.Send <- fsm.OrderEvent{Floor: order.Order.Floor,
					Button: order.Order.Type, TurnLightOn: true}
			} else {
				addHallOrder_fsmCh.Send <- fsm.OrderEvent{Floor: order.Order.Floor,
					Button: order.Order.Type, TurnLightOn: false}
			}
			assignedOrders[order.Order.ID] = order.Order

		case msg, _ := <-completedHallOrdersThisElev_fsmCh.Recv:
			completedOrders := msg.([]fsm.OrderEvent)

			// find and remove all equivalent placedOrders
			for _, completedOrder := range completedOrders {
				orderID := createOrderID(completedOrder.Floor, completedOrder.Button, fsm.N_FLOORS)

				completedOrder_commhandlerCh.Send <- msgs.Order{ID: orderID, Floor: completedOrder.Floor, Type: completedOrder.Button}
				Info.Printf("completed order %v\n", orderID)

				//delete order
				delete(assignedOrders, orderID)
				delete(acceptedOrders, orderID)
				delete(chosenElevatorForOrder, orderID)
				delete(placedOrders, orderID)
			}

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
			for _, order := range assignedOrders {
				if order.Floor == completedOrder.Floor &&
					order.Type == completedOrder.Type {
					delete(assignedOrders, order.ID)
				}
			}

			deleteHallOrder_fsmCh.Send <- fsm.OrderEvent{Floor: completedOrder.Floor, Button: completedOrder.Type}

		case msg, _ := <-downedElevators_commhandlerCh.Recv:
			downedElevators := msg.([]msgs.Heartbeat)

			for _, lastHeartbeat := range downedElevators {
				// elevator is down
				Info.Printf("down: %+v %v\n", lastHeartbeat.SenderID, lastHeartbeat.AcceptedOrders)
				// Add taken orders
				for orderID, order := range lastHeartbeat.TakenOrders {
					assignedOrders[orderID] = order
					addHallOrder_fsmCh.Send <- fsm.OrderEvent{Floor: order.Floor, Button: order.Type, TurnLightOn: false}
				}
				// Add accepted orders
				for orderID, order := range lastHeartbeat.AcceptedOrders {
					acceptedOrders[orderID] = order
					chosenElevatorForOrder[orderID] = lastHeartbeat.ChosenElevatorForOrder[orderID]
					addHallOrder_fsmCh.Send <- fsm.OrderEvent{Floor: order.Floor, Button: order.Type, TurnLightOn: true}
				}
				delete(elevators, lastHeartbeat.SenderID)
			}

		case msg, _ := <-elevatorStatus_fsmCh.Recv:
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
			for k, v := range assignedOrders {
				takenOrdersDeepCopy[k] = v
			}

			// build heartbeat
			heartbeat := msgs.Heartbeat{SenderID: thisID,
				Status:                 elevatorStatus,
				AcceptedOrders:         acceptedOrdersDeepCopy,
				ChosenElevatorForOrder: chosenElevatorForOrderDeepCopy,
				TakenOrders:            takenOrdersDeepCopy}

			thisElevatorHeartbeat_commhandlerCh.Send <- heartbeat

		case msg, _ := <-allElevatorsHeartbeat_commhandlerCh.Recv:
			allElevatorsHeartbeat := msg.([]msgs.Heartbeat)
			// update elevators
			for _, elevatorHeartbeat := range allElevatorsHeartbeat {
				elevators[elevatorHeartbeat.SenderID] = elevatorHeartbeat
			}
			// update lights
			var updateLights [fsm.N_FLOORS][fsm.N_BUTTONS]bool
			for _, elevatorHeartbeat := range allElevatorsHeartbeat {
				for _, acceptedOrder := range elevatorHeartbeat.AcceptedOrders {
					chosenElevatorID := elevatorHeartbeat.ChosenElevatorForOrder[acceptedOrder.ID]
					// find heartbeat for chosenElevatorID in allElevatorsHeartbeat
					for _, chosenElevatorHeartbeat := range allElevatorsHeartbeat {
						if chosenElevatorHeartbeat.SenderID == chosenElevatorID {
							chosenElevatorTakenOrder := chosenElevatorHeartbeat.TakenOrders
							chosenElevatorStatus := chosenElevatorHeartbeat.Status
							if _, exists := chosenElevatorTakenOrder[acceptedOrder.ID]; exists &&
								chosenElevatorStatus.Orders[acceptedOrder.Floor][acceptedOrder.Type] {
								updateLights[acceptedOrder.Floor][acceptedOrder.Type] = true
								break
							}
						}
					}
				}
			}
			updateLights_fsmCh.Send <- updateLights

		}
	}
}
