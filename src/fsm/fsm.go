package fsm

import (
	"fmt"
	"sync"
	"time"

	"../elevio"
	"../go-nonblockingchan"
)

const N_FLOORS = 4
const N_BUTTONS = 3
const DOOR_OPEN_TIME = 3.0

type State int

const (
	IDLE State = iota
	DRIVE
	DOOR_OPEN
)

type Elevator struct {
	Floor           int
	Dir             elevio.MotorDirection
	Orders          [N_FLOORS][N_BUTTONS]bool
	CompletedOrders [N_FLOORS][N_BUTTONS]bool
	Lights          [N_FLOORS][N_BUTTONS]bool
	State           State
}

type OrderEvent struct {
	Floor       int
	Button      elevio.ButtonType
	TurnLightOn bool
}

func FSM(elevServerAddr string, addHallOrderCh *nbc.NonBlockingChan, deleteHallOrderCh *nbc.NonBlockingChan,
	placedHallOrderCh *nbc.NonBlockingChan, completedHallOrderCh *nbc.NonBlockingChan,
	elevatorStatusCh *nbc.NonBlockingChan, updateLightsCh *nbc.NonBlockingChan, wg_ptr *sync.WaitGroup) {

	//fmt.Println("[fsm]: starting")
	elevio.Init(elevServerAddr, N_FLOORS)
	//fmt.Println("Hardware initialized")
	buttonCh := make(chan elevio.ButtonEvent)
	floorSensorCh := make(chan int)

	var elevator Elevator

	var doorTimer = time.NewTimer(DOOR_OPEN_TIME * time.Second)
	doorTimer.Stop()

	go elevio.PollFloorSensor(floorSensorCh)
	initializeState(&elevator, floorSensorCh)
	go elevio.PollButtons(buttonCh)

	// Wait until all modules are initialized
	wg_ptr.Done()
	fmt.Println("[fsm]: initialized")
	wg_ptr.Wait()
	fmt.Println("[fsm]: starting")

	elevatorStatusCh.Send <- elevator
	for {
		select {

		case buttonEvent := <-buttonCh:
			if buttonEvent.Button == elevio.BT_Cab {
				elevator.Orders[buttonEvent.Floor][buttonEvent.Button] = true
				elevator.Lights[buttonEvent.Floor][buttonEvent.Button] = true
				elevio.SetButtonLamp(buttonEvent.Button, buttonEvent.Floor, true)
				//fmt.Println("[fsm]: Cab order added and lights turned on")
				//fmt.Println("[fsm]: Estimated completion time: %f", EstimatedCompletionTime(elevator, OrderEvent{buttonEvent.Floor, buttonEvent.Button, false}))
				switch elevator.State {
				case IDLE:
					if shouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
						setStateToDoorOpen(&elevator, doorTimer)
						clearOrdersAtFloor(&elevator, false)
					} else {
						updateElevatorDirection(&elevator)
						setStateToDrive(&elevator)
					}
				case DOOR_OPEN: // a new order -> extend timer, determine direction
					if shouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
						setStateToDoorOpen(&elevator, doorTimer)
						clearOrdersAtFloor(&elevator, false)
					} else {
						updateElevatorDirection(&elevator)
					}
				}
			} else {
				placedHallOrderCh.Send <- OrderEvent{Floor: buttonEvent.Floor, Button: buttonEvent.Button, TurnLightOn: false}
			}

		case msg, _ := <-addHallOrderCh.Recv:
			hallOrder := msg.(OrderEvent)
			elevator.Orders[hallOrder.Floor][hallOrder.Button] = true
			enableLight := hallOrder.TurnLightOn || elevator.Lights[hallOrder.Floor][hallOrder.Button]
			elevio.SetButtonLamp(hallOrder.Button, hallOrder.Floor, enableLight)
			elevator.Lights[hallOrder.Floor][hallOrder.Button] = enableLight

			//fmt.Println("[fsm]: Estimated completion time: %f", EstimatedCompletionTime(elevator, hallOrder))
			switch elevator.State {
			case IDLE:
				if shouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
					setStateToDoorOpen(&elevator, doorTimer)
					clearOrdersAtFloor(&elevator, false)
					fmt.Println("[fsm]: Door opens")
				} else {
					updateElevatorDirection(&elevator)
					setStateToDrive(&elevator)
				}
			case DOOR_OPEN: // a new order -> extend timer, determine direction
				if shouldOpenDoor(elevator) { //buttonEvent.Floor == last_floor
					setStateToDoorOpen(&elevator, doorTimer)
					clearOrdersAtFloor(&elevator, false)
					////fmt.Println("Door keeps open")
				} else {
					updateElevatorDirection(&elevator)
				}
			default:
				updateElevatorDirection(&elevator)
			}

		case msg, _ := <-deleteHallOrderCh.Recv:
			hallOrder := msg.(OrderEvent)

			elevator.Orders[hallOrder.Floor][hallOrder.Button] = false
			elevator.CompletedOrders[hallOrder.Floor][hallOrder.Button] = false //necessary ???
			elevio.SetButtonLamp(hallOrder.Button, hallOrder.Floor, false)
			elevator.Lights[hallOrder.Floor][hallOrder.Button] = false
			if elevator.State == DOOR_OPEN {
				updateElevatorDirection(&elevator)
				clearOrdersAtFloor(&elevator, false)
			}

		case elevator.Floor = <-floorSensorCh: //new floor reached -> door_open, idle, drive in other direction, continue drive
			//fmt.Println("[fsm]: New floor reached")
			elevio.SetFloorIndicator(elevator.Floor)
			updateElevatorDirection(&elevator)
			if shouldOpenDoor(elevator) {
				fmt.Printf("[fsm]: elevatorStatus before opening door: %v", elevator)
				setStateToDoorOpen(&elevator, doorTimer)
				clearOrdersAtFloor(&elevator, false)
				fmt.Printf("[fsm]: elevatorStatus after opening door: %v", elevator)
			} else if elevator.Dir == elevio.MD_Stop {
				setStateToIdle(&elevator)
			} else {
				setStateToDrive(&elevator)
			}

		case <-doorTimer.C:
			////fmt.Println("Door closes")
			elevio.SetDoorOpenLamp(false)
			updateElevatorDirection(&elevator)
			if elevator.Dir == elevio.MD_Stop {
				setStateToIdle(&elevator)
			} else {
				setStateToDrive(&elevator)
			}

		case msg, _ := <-updateLightsCh.Recv:
			updateLights := msg.([N_FLOORS][N_BUTTONS]bool)

			for floor := 0; floor < N_FLOORS; floor++ {
				for button := 0; button < N_BUTTONS-1; button++ { // note ignoring cab call lights
					if !(floor == N_FLOORS-1 && elevio.ButtonType(button) == elevio.BT_HallUp) &&
						!(floor == 0 && elevio.ButtonType(button) == elevio.BT_HallDown) {
						elevator.Lights[floor][button] = updateLights[floor][button]
						elevio.SetButtonLamp(elevio.ButtonType(button), floor, elevator.Lights[floor][button])
					}
				}
			}
		}
		var completedHallOrderSlice []OrderEvent
		for floor := 0; floor < N_FLOORS; floor++ {
			for button := 0; button < N_BUTTONS; button++ {
				if elevio.ButtonType(button) != elevio.BT_Cab && elevator.CompletedOrders[floor][button] {
					completedHallOrderSlice = append(completedHallOrderSlice, OrderEvent{Floor: floor, Button: elevio.ButtonType(button), TurnLightOn: false})
				}
				elevator.CompletedOrders[floor][button] = false
			}
		}
		if len(completedHallOrderSlice) > 0 {
			fmt.Printf("[fsm]: completedHallOrderSlice: %v", completedHallOrderSlice)
			completedHallOrderCh.Send <- completedHallOrderSlice
		}
		elevatorStatusCh.Send <- elevator
	}
}

func initializeState(elev *Elevator, floorSensorCh <-chan int) {
	for f := 0; f < N_FLOORS; f++ {
		for b := 0; b < 3; b++ {
			elevio.SetButtonLamp(elevio.ButtonType(b), f, false)
		}
	}
	elevio.SetMotorDirection(elevio.MD_Down)
	elev.Floor = <-floorSensorCh
	elev.Dir = elevio.MD_Stop
	setStateToIdle(elev)
	elevio.SetFloorIndicator(elev.Floor)
}

func setStateToDoorOpen(elev *Elevator, doorTimer *time.Timer) {
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

func shouldOpenDoor(elev Elevator) bool {
	switch elev.Dir {
	case elevio.MD_Up:

	}
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
		elev.CompletedOrders[elev.Floor][buttonType] = true
		elev.Lights[elev.Floor][buttonType] = false
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

func EstimatedCompletionTime(elev Elevator, orderEvent OrderEvent) float64 { // TO DO
	var TRAVEL_TIME = 2.5
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
		if shouldOpenDoor(elev) {
			duration += DOOR_OPEN_TIME
			clearOrdersAtFloor(&elev, true)
			updateElevatorDirection(&elev)
			if elev.Dir == elevio.MD_Stop || duration > 60.0 { // TO DO
				////fmt.Println("Duration until completion %f", duration)
				return duration
			}
		}
		elev.Floor += int(elev.Dir)
		duration += TRAVEL_TIME
		//////fmt.Println("Duration until now %f", duration)
	}
}
