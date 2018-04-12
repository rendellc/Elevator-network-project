//package order_handler
package order

import (
	"../elevio"
	"../fsm"
	"bufio"
	"fmt"
	"os"
	"strconv"
)

func createOrderID(floor int, button elevio.ButtonType, id string, max_floors int, max_id int) int {
	return max_id*(max_floors*button+floor) + strconv.Atoi(id)
}

func OrderHandler(id string,
	/* Read channels */
	elevatorStatusCh <-chan fsm.Elevator,
	allElevatorsHeartbeatCh <-chan msgs.Heartbeat,
	placedHallOrderCh <-chan msgs.Order,
	safeOrderCh <-chan msgs.SafeOrderMsg,
	thisTakeOrderCh <-chan msgs.TakeOrderMsg,
	downedElevatorsCh <-chan []msgs.Heartbeat,
	completedOrderThisElevCh <-chan msgs.Order,
	completedOrderOtherElevCh <-chan msgs.Order,
	/* Write channels */
	addHallOrderCh chan<- fsm.OrderEvent,
	broadcastTakeOrderCh chan<- msgs.TakeOrderMsg,
	placedOrderCh chan<- msgs.Order,
	deleteHallOrderCh chan<- fsm.OrderEvent,
	completedOrderCh chan<- msgs.Order,
	thisElevatorHeartbeatCh chan<- msgs.Heartbeat,
	turnOnLightsCh chan<- [N_FLOORS][N_BUTTONS]bool,
	/* Sync */
	wg *sync.WaitGroup) {

	var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

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
		case msg := <-thisTakeOrderCh:
			// TODO: Verify that only other elevators orders come in here. Or else lights will be wrong
			takenOrders[msg.Order.ID] = msg.Order
			addHallOrderCh <- fsm.OrderEvent{msg.Order.Floor, msg.Order.Type, false}

		case safeMsg := <-safeOrderCh:
			fmt.Printf("[safeOrderCh]: %v\n", safeMsg)
			if safeMsg.ReceiverID == thisID {
				if order, exists := placedOrders[safeMsg.Order.ID]; exists {
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
					broadcastTakeOrderCh <- takeOrderMsg

					if bestID == thisID {
						takenOrders[order.ID] = order
						addHallOrderCh <- fsm.OrderEvent{Floor: order.Floor,
							Button:  order.Type,
							LightOn: true}
					}
				} else {
					fmt.Println("[orderHandler]: order didn't exist")
				}
			}

		case downedElevators := <-downedElevatorsCh:
			for _, lastHeartbeat := range downedElevators {
				// elevator is down
				fmt.Printf("[orderHandler]: Down: %+v %v\n", lastHeartbeat.SenderID, lastHeartbeat.AcceptedOrders)
				// Add taken orders
				for orderID, order := range lastHeartbeat.TakenOrders {
					takenOrders[orderID] = order
					addHallOrderCh <- fsm.OrderEvent{order.Floor, order.Type, false}
				}
				// Add accepted orders
				for orderID, order := range lastHe_artbeat.AcceptedOrders {
					acceptedOrders[orderID] = order
					addHallOrderCh <- fsm.OrderEvent{order.Floor, order.Type, true}
				}
				delete(elevators, lastHeartbeat.SenderID) // Not sure about this ? can be handled by heartbeat channel
			}

		case buttonEvent := <-placedHallOrderCh:
			// order : contains all placedOrders placed at the elevator. What happens if an order is completed, rejected ?
			orderID := createOrderID(buttonEvent.Floor, buttonEvent.Type, thisID, N_FLOORS, 256)
			order := msgs.Order{ID: orderID, Floor: buttonEvent.Floor, Type: buttonEvent.Button}
			placedOrders[orderID] = order
			placedOrderCh <- order

		case completedOrder := <-completedOrderOtherElevCh:
			for _, order := range placedOrders {
				if order.Floor == completedOrder.Floor &&
					order.Type == completedOrder.Button {
					delete(placedOrders, order.ID)
				}
			}
			for _, order := range acceptedOrders {
				if order.Floor == completedOrder.Floor &&
					order.Type == completedOrder.Button {
					delete(acceptedOrders, order.ID)
				}
			}
			for _, order := range takenOrders {
				if order.Floor == completedOrder.Floor &&
					order.Type == completedOrder.Button {
					delete(takenOrders, order.ID)
				}
			}
			deleteHallOrderCh <- fsm.OrderEvent{completedOrder.Floor, completedOrder.Type}

		case completedOrders := <-completedOrderThisElevCh:
			// find and remove all equivalent placedOrders
			for _, completedOrder := range completedOrders {
				orderID := createOrderID(completedOrder.Floor, completedOrder.ButtonType, thisID, N_FLOORS, 256)
				fmt.Printf("[orderHandler]: completed order %v\n", orderID)
				// broadcast to network that order is completed
				if order, exists := takenOrders[orderID]; exists {
					completedOrderCh <- order
				} else {
					fmt.Println("[orderHandler]: ERROR: completed non-taken order")
				}
				//delete order
				delete(takenOrders, orderID)
				delete(acceptedOrders, orderID)
				delete(placedOrders, orderID)
			}

		case elevatorStatus := <-elevatorStatusCh:
			// build heartbeat
			heartbeat := msgs.Heartbeat{SenderID: thisID,
				Status:         elevatorStatus,
				AcceptedOrders: acceptedOrders,
				TakenOrders:    takenOrders}

			//fmt.Println("[network]: writing to thisElevatorHeartbeatCh")
			thisElevatorHeartbeatCh <- heartbeat
			//fmt.Println("[network]: thisElevatorHeartbeatCh done")

		case allElevatorsHeartbeat := <-allElevatorsHeartbeatCh:

			var turnOnLights [N_FLOORS][N_BUTTONS]bool
			for _, elevatorHeartbeat := range allElevatorsHeartbeat {
				elevators[elevatorHeartbeat.SenderID] = elevatorHeartbeat
				if elevatorHeartbeat.SenderID != thisID {
					for _, acceptedOrder := range elevatorHeartbeat.AcceptedOrders {
						turnOnLights[acceptedOrder.Floor][acceptedOrder.Type] = true
					}
				}
			}
			turnOnLightsCh <- turnOnLights

			// debug
			var elevatorIDList []string
			for id, _ := range elevators {
				elevatorIDList = append(elevatorIDList, id)
			}
			fmt.Printf("[orderHandler]: elevators: %v\n", elevatorIDList)

		case <-time.After(15 * time.Second):
			// an (empty) event every second, avoids some forms of locking
			//fmt.Println("[orderHandler]: running")
			var orderList []msgs.Order
			for orderID, _ := range acceptedOrders {
				if order, exists := orders[orderID]; !exists {
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
