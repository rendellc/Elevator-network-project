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
	Orders [N_FLOORS][N_BUTTONS]bool //type OrderBook map[int]map[int]bool
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
	//fmt.Println("Initializing")
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

func clearOrder(elev *Elevator, buttonType elevio.ButtonType, simulationMode bool) {
	if elev.Orders[elev.Floor][buttonType] {
		elev.Orders[elev.Floor][buttonType] = false
		if !simulationMode {
			elevio.SetButtonLamp(buttonType, elev.Floor, false)
		}
	}
}

func clearOrdersAtFloor(elev *Elevator, simulationMode bool) {
	if elev.Dir == elevio.MD_Up {
		clearOrder(elev, elevio.BT_HallUp, simulationMode)
		clearOrder(elev, elevio.BT_Cab, simulationMode)
		if !isOrderAbove(*elev) {
			clearOrder(elev, elevio.BT_HallDown, simulationMode)
		}
	} else if elev.Dir == elevio.MD_Down {
		clearOrder(elev, elevio.BT_HallDown, simulationMode)
		clearOrder(elev, elevio.BT_Cab, simulationMode)
		if !isOrderBelow(*elev) {
			clearOrder(elev, elevio.BT_HallUp, simulationMode)
		}
	} else {
		clearOrder(elev, elevio.BT_HallUp, simulationMode)
		clearOrder(elev, elevio.BT_HallDown, simulationMode)
		clearOrder(elev, elevio.BT_Cab, simulationMode)
	}
}

func FSM(simAddr string, addHallOrderCh <-chan OrderEvent, deleteHallOrderCh <-chan OrderEvent,
	placedHallOrderCh chan<- OrderEvent, completedHallOrderCh chan<- []OrderEvent,
	elevatorStatusCh chan<- Elevator, turnOnLightsCh <-chan [N_FLOORS][N_BUTTONS]bool) {

	fmt.Println("[fsm]: starting")
	elevio.Init(simAddr, N_FLOORS)
	//fmt.Println("Hardware initialized")
	buttonCh := make(chan elevio.ButtonEvent)
	floorSensorCh := make(chan int)
	var elevator Elevator
	prev := elevator

	//doorTimer.Stop()
	go elevio.PollFloorSensor(floorSensorCh)
	initializeState(&elevator, floorSensorCh)
	go elevio.PollButtons(buttonCh)
	fmt.Println("[fsm]: ready")
	for {
		select {
		case buttonEvent := <-buttonCh:
			if buttonEvent.Button == elevio.BT_Cab {
				elevator.Orders[buttonEvent.Floor][buttonEvent.Button] = true
				elevio.SetButtonLamp(buttonEvent.Button, buttonEvent.Floor, true)
				fmt.Println("[fsm]: Cab order added and lights turned on")
				fmt.Println("[fsm]: Estimated completion time: %f", EstimatedCompletionTime(elevator, OrderEvent{buttonEvent.Floor, buttonEvent.Button, false}))
				switch elevator.State {
				case IDLE:
					if elevShouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
						setStateToDoorOpen(&elevator)
						clearOrdersAtFloor(&elevator, false)
					} else {
						updateElevatorDirection(&elevator)
						setStateToDrive(&elevator)
					}
				case DOOR_OPEN: // a new order -> extend timer, determine direction
					if elevShouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
						setStateToDoorOpen(&elevator)
						clearOrdersAtFloor(&elevator, false)
					} else {
						updateElevatorDirection(&elevator)
					}
				}
			} else {
				//fmt.Println("Button Event: Hall order")
				placedHallOrderCh <- OrderEvent{buttonEvent.Floor, buttonEvent.Button, false}
			}

		case hallOrder := <-addHallOrderCh:
			elevator.Orders[hallOrder.Floor][hallOrder.Button] = true
			elevio.SetButtonLamp(hallOrder.Button, hallOrder.Floor, hallOrder.TurnLightOn)
			fmt.Println("[fsm]: Hall order added and lights turned on if requested")
			fmt.Println("[fsm]: Estimated completion time: %f", EstimatedCompletionTime(elevator, hallOrder))
			switch elevator.State {
			case IDLE:
				if elevShouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
					setStateToDoorOpen(&elevator)
					clearOrdersAtFloor(&elevator, false)
					//fmt.Println("Door opens")
				} else {
					updateElevatorDirection(&elevator)
					setStateToDrive(&elevator)
					//fmt.Println("Elevator begins to move")
				}
			case DOOR_OPEN: // a new order -> extend timer, determine direction
				if elevShouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
					setStateToDoorOpen(&elevator)
					clearOrdersAtFloor(&elevator, false)
					//fmt.Println("Door keeps open")
				} else {
					updateElevatorDirection(&elevator)
				}
			}

		case hallOrder := <-deleteHallOrderCh:
			fmt.Printf("[fsm]: Delete order %+v\n", hallOrder)

			// TODO: Maybe validate order here? (check that hallOrder.Floor is reasonable) So that invalid hallOrder doesn't cause index out of bounds crash

			elevator.Orders[hallOrder.Floor][hallOrder.Button] = false
			elevio.SetButtonLamp(hallOrder.Button, hallOrder.Floor, false)
			if elevator.State == DOOR_OPEN {
				updateElevatorDirection(&elevator)
				clearOrdersAtFloor(&elevator, false)
			}

		case elevator.Floor = <-floorSensorCh: //new floor reached -> door_open, idle, drive in other direction, continue drive
			fmt.Println("[fsm]: New floor reached")
			elevio.SetFloorIndicator(elevator.Floor)
			updateElevatorDirection(&elevator)
			if elevShouldOpenDoor(elevator) {
				setStateToDoorOpen(&elevator)
				clearOrdersAtFloor(&elevator, false)
			} else if elevator.Dir == elevio.MD_Stop {
				setStateToIdle(&elevator)
			} else {
				setStateToDrive(&elevator)
			}

		case <-doorTimer.C:
			//fmt.Println("Door closes")
			elevio.SetDoorOpenLamp(false)
			updateElevatorDirection(&elevator)
			if elevator.Dir == elevio.MD_Stop {
				setStateToIdle(&elevator)
			} else {
				setStateToDrive(&elevator)
			}

		case turnOnLights:= <- turnOnLightsCh:
			for floor := 0; floor < N_FLOORS; floor++ {
				for button := 0; button < N_BUTTONS; button++ {
					if turnOnLights[floor][button] {
						elevio.SetButtonLamp(elevio.ButtonType(button), floor, true)
					}
				}
			}
		}
		if prev != elevator {
			var completedHallOrderSlice []OrderEvent
			for floor := 0; floor < N_FLOORS; floor++ {
				for button := 0; button < N_BUTTONS; button++ {
					if !elevator.Orders[floor][button] && prev.Orders[floor][button] {
						completedHallOrderSlice = append(completedHallOrderSlice, OrderEvent{floor, elevio.ButtonType(button), false})
					}
				}
			}
			if len(completedHallOrderSlice) > 0 {
				completedHallOrderCh <- completedHallOrderSlice
				}
			elevatorStatusCh <- elevator
			prev = elevator
		}
	}
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
		duration -= DOOR_OPEN_TIME / 2
		updateElevatorDirection(&elev)
		if elev.Dir == elevio.MD_Stop {
			return duration
		}
		//duration -= DOOR_OPEN_TIME
		//elev.Orders[elev.Floor][elevio.BT_Cab] = true
	}
	for {
		if elevShouldOpenDoor(elev) {
			duration += DOOR_OPEN_TIME
			clearOrdersAtFloor(&elev, true)
			updateElevatorDirection(&elev)
			if elev.Dir == elevio.MD_Stop || duration > 60.0 { // TO DO
				//fmt.Println("Duration until completion %f", duration)
				return duration
			}
		}
		elev.Floor += int(elev.Dir)
		duration += TRAVEL_TIME
		////fmt.Println("Duration until now %f", duration)
	}
}
