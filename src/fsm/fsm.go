package fsm

import (
	"../elevio"
	"fmt"
	"time"
	//"../order_handler"
)

type State int

const (
	IDLE State = iota
	DRIVE
	DOOR_OPEN
)

type Elevator struct {
	Floor  int
	Dir    elevio.MotorDirection
	Orders [N_FLOORS][N_BUTTONS]bool
	State  State
}

type OrderEvent struct {
	Floor       int
	Button      elevio.ButtonType
	TurnLightOn bool
}

const N_FLOORS = 4 //import
const N_BUTTONS = 3
const TRAVEL_TIME = 2.5
const DOOR_OPEN_TIME = 3.0

var doorTimer = time.NewTimer(DOOR_OPEN_TIME * time.Second)

func initializeState(elev *Elevator, floorSensorCh <-chan int) {
	// Add timer? After timer goes out, then drive down.
	fmt.Println("Initializing")
	elevio.SetMotorDirection(elevio.MD_Down)
	elev.Floor = <-floorSensorCh
	elev.Dir = elevio.MD_Stop
	setStateToIdle(elev)
	elevio.SetFloorIndicator(elev.Floor)
}

func setStateToDoorOpen(elev *Elevator) {
	elev.State = DOOR_OPEN
	elevio.SetMotorDirection(elevio.MD_Stop)
	elevio.SetDoorOpenLamp(true)
	doorTimer.Reset(DOOR_OPEN_TIME * time.Second)
}

func setStateToDrive(elev *Elevator) {
	elev.State = DRIVE
	elevio.SetMotorDirection(elev.Dir)
}

func setStateToIdle(elev *Elevator) {
	elev.State = IDLE
	elevio.SetMotorDirection(elev.Dir)
}

func isOrderAbove(elev Elevator) bool {
	for floor := elev.Floor + 1; floor < N_FLOORS; floor++ {
		for button := 0; button < N_BUTTONS; button++ {
			if elev.Orders[floor][button] {
				return true
			}
		}
	}
	return false
}

func isOrderBelow(elev Elevator) bool {
	for floor := 0; floor < elev.Floor; floor++ {
		for button := 0; button < N_BUTTONS; button++ {
			if elev.Orders[floor][button] {
				return true
			}
		}
	}
	return false
}

func elevShouldOpenDoor(elev Elevator) bool {
	if elev.Dir == elevio.MD_Up {
		if elev.Orders[elev.Floor][elevio.BT_Cab] ||
			elev.Orders[elev.Floor][elevio.BT_HallUp] ||
			!isOrderAbove(elev) && elev.Orders[elev.Floor][elevio.BT_HallDown] {
			return true
		}
	} else if elev.Dir == elevio.MD_Down {
		if elev.Orders[elev.Floor][elevio.BT_Cab] ||
			elev.Orders[elev.Floor][elevio.BT_HallDown] ||
			!isOrderBelow(elev) && elev.Orders[elev.Floor][elevio.BT_HallUp] {
			return true
		}
	} else { //different from files online
		if elev.Orders[elev.Floor][elevio.BT_Cab] ||
			elev.Orders[elev.Floor][elevio.BT_HallUp] ||
			elev.Orders[elev.Floor][elevio.BT_HallDown] {
			return true
		}
	}
	return false
}

// Intern order handler
func updateElevatorDirection(elev *Elevator) {
	if elev.Dir == elevio.MD_Up {
		if !isOrderAbove(*elev) {
			if isOrderBelow(*elev) {
				elev.Dir = elevio.MD_Down
			} else {
				elev.Dir = elevio.MD_Stop
			}
		}
	} else if elev.Dir == elevio.MD_Down {
		if !isOrderBelow(*elev) {
			if isOrderAbove(*elev) {
				elev.Dir = elevio.MD_Up
			} else {
				elev.Dir = elevio.MD_Stop
			}
		}
	} else {
		if isOrderAbove(*elev) {
			elev.Dir = elevio.MD_Up
		} else if isOrderBelow(*elev) {
			elev.Dir = elevio.MD_Down
		}
	}
}

func clearOrdersAtFloor(elev *Elevator) {
	if elev.Dir == elevio.MD_Up {
		elev.Orders[elev.Floor][elevio.BT_HallUp] = false
		elev.Orders[elev.Floor][elevio.BT_Cab] = false
		if !isOrderAbove(*elev) {
			elev.Orders[elev.Floor][elevio.BT_HallDown] = false
		}
	} else if elev.Dir == elevio.MD_Down {
		elev.Orders[elev.Floor][elevio.BT_HallDown] = false
		elev.Orders[elev.Floor][elevio.BT_Cab] = false
		if !isOrderBelow(*elev) {
			elev.Orders[elev.Floor][elevio.BT_HallUp] = false
		}
	} else {
		elev.Orders[elev.Floor][elevio.BT_HallUp] = false
		elev.Orders[elev.Floor][elevio.BT_HallDown] = false
		elev.Orders[elev.Floor][elevio.BT_Cab] = false
	}
}

func clearLightsAtFloor(elev Elevator) { // ???
	if elev.Dir == elevio.MD_Up {
		elevio.SetButtonLamp(elevio.BT_HallUp, elev.Floor, false)
		elevio.SetButtonLamp(elevio.BT_Cab, elev.Floor, false)
		if !isOrderAbove(elev) {
			elevio.SetButtonLamp(elevio.BT_HallDown, elev.Floor, false)
		}
	} else if elev.Dir == elevio.MD_Down {
		elevio.SetButtonLamp(elevio.BT_HallDown, elev.Floor, false)
		elevio.SetButtonLamp(elevio.BT_Cab, elev.Floor, false)
		if !isOrderBelow(elev) {
			elevio.SetButtonLamp(elevio.BT_HallUp, elev.Floor, false)
		}
	} else {
		elevio.SetButtonLamp(elevio.BT_HallUp, elev.Floor, false)
		elevio.SetButtonLamp(elevio.BT_HallDown, elev.Floor, false)
		elevio.SetButtonLamp(elevio.BT_Cab, elev.Floor, false)
	}
}

func FSM(simAddr string, addHallOrderCh <-chan OrderEvent, deleteHallOrderCh <-chan OrderEvent,
	placedHallOrderCh chan<- OrderEvent, completedHallOrderCh chan<- OrderEvent,
	elevatorStatusCh chan<- Elevator) {

	fmt.Println("Lets start")
	elevio.Init(simAddr, N_FLOORS)
	//elevio.Init("localhost:15657", N_FLOORS)
	fmt.Println("Hardware initialized")
	buttonCh := make(chan elevio.ButtonEvent)
	floorSensorCh := make(chan int)
	var elevator Elevator
	prev := elevator

	//doorTimer.Stop()
	go elevio.PollFloorSensor(floorSensorCh)
	initializeState(&elevator, floorSensorCh)
	go elevio.PollButtons(buttonCh)
	fmt.Println("Before for loop")

	for {
		select {
		case buttonEvent := <-buttonCh:
			fmt.Println("Button Event")
			if buttonEvent.Button == elevio.BT_Cab {
				fmt.Println("Button Event: Cab order")
				elevator.Orders[buttonEvent.Floor][buttonEvent.Button] = true
				elevio.SetButtonLamp(buttonEvent.Button, buttonEvent.Floor, true)
				fmt.Println("Cab order added and lights turned on")
				fmt.Println("Estimated completion time: %f", EstimatedCompletionTime(elevator, OrderEvent{buttonEvent.Floor, buttonEvent.Button, false}))
				switch elevator.State {
				case IDLE:
					if elevShouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
						setStateToDoorOpen(&elevator)
						clearOrdersAtFloor(&elevator)
						clearLightsAtFloor(elevator)
						fmt.Println("Door opens")
					} else {
						updateElevatorDirection(&elevator)
						setStateToDrive(&elevator)
						fmt.Println("Elevator begins to move")
					}
				case DOOR_OPEN: // a new order -> extend timer, determine direction
					if elevShouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
						setStateToDoorOpen(&elevator)
						clearOrdersAtFloor(&elevator)
						clearLightsAtFloor(elevator)
						fmt.Println("Door keeps open")
					} else {
						updateElevatorDirection(&elevator)
					}
				}
			} else {
				fmt.Println("Button Event: Hall order")
				placedHallOrderCh <- OrderEvent{buttonEvent.Floor, buttonEvent.Button, false}
			}

		case hallOrder := <-addHallOrderCh:
			fmt.Println("Hall order")
			elevator.Orders[hallOrder.Floor][hallOrder.Button] = true
			elevio.SetButtonLamp(hallOrder.Button, hallOrder.Floor, hallOrder.TurnLightOn)
			fmt.Println("Hall order added and lights turned on if requested")
			fmt.Println("Estimated completion time: %f", EstimatedCompletionTime(elevator, hallOrder))
			switch elevator.State {
			case IDLE:
				if elevShouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
					setStateToDoorOpen(&elevator)
					clearOrdersAtFloor(&elevator)
					clearLightsAtFloor(elevator)
					fmt.Println("Door opens")
				} else {
					updateElevatorDirection(&elevator)
					setStateToDrive(&elevator)
					fmt.Println("Elevator begins to move")
				}
			case DOOR_OPEN: // a new order -> extend timer, determine direction
				if elevShouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
					setStateToDoorOpen(&elevator)
					clearOrdersAtFloor(&elevator)
					clearLightsAtFloor(elevator)
					fmt.Println("Door keeps open")
				} else {
					updateElevatorDirection(&elevator)
				}
			}

		case hallOrder := <-deleteHallOrderCh:
			fmt.Println("Delete order: Not implemented", hallOrder)

		case elevator.Floor = <-floorSensorCh: //new floor reached -> door_open, idle, drive in other direction, continue drive
			fmt.Println("New floor reached")
			elevio.SetFloorIndicator(elevator.Floor)
			if elevShouldOpenDoor(elevator) {
				setStateToDoorOpen(&elevator)
				updateElevatorDirection(&elevator)
				clearOrdersAtFloor(&elevator)
				clearLightsAtFloor(elevator)
			}
		// when delete order feature added: continue driving? drive in opossite direction? go to idle

		case <-doorTimer.C:
			fmt.Println("Door closes")
			elevio.SetDoorOpenLamp(false)
			updateElevatorDirection(&elevator)
			if elevator.Dir == elevio.MD_Stop {
				setStateToIdle(&elevator)
			} else {
				setStateToDrive(&elevator)
			}
		}
		if prev != elevator {
			elevatorStatusCh <- elevator
			prev = elevator
		}
	}
	fmt.Println("Lets end")
}

func EstimatedCompletionTime(elev Elevator, orderEvent OrderEvent) float64 { // TO DO
	duration := 0.0
	elev.Orders[orderEvent.Floor][orderEvent.Button] = true
	switch elev.State {
	case IDLE:
		updateElevatorDirection(&elev)
		if elev.Dir == elevio.MD_Stop {
			return duration
		}
	case DRIVE:
		duration += TRAVEL_TIME / 2
		elev.Floor += int(elev.Dir)
	case DOOR_OPEN:
		//duration -= DOOR_OPEN_TIME/2
		//updateElevatorDirection(&elev)
		//if(elev.Dir == elevio.MD_Stop){
		//  return duration
		//}
		duration -= DOOR_OPEN_TIME
		elev.Orders[elev.Floor][elevio.BT_Cab] = true
	}
	for {
		if elevShouldOpenDoor(elev) {
			duration += DOOR_OPEN_TIME
			clearOrdersAtFloor(&elev)
			updateElevatorDirection(&elev)
			if elev.Dir == elevio.MD_Stop || duration > 60.0 { // TO DO
				fmt.Println("Duration until completion %f", duration)
				return duration
			}
		}
		elev.Floor += int(elev.Dir)
		duration += TRAVEL_TIME
		//fmt.Println("Duration until now %f", duration)
	}
}
