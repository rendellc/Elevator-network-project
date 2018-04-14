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

	var currElevator Elevator
	prevElevator := currElevator

	var doorTimer = time.NewTimer(DOOR_OPEN_TIME * time.Second)
	doorTimer.Stop()

	go elevio.PollFloorSensor(floorSensorCh)
	initializeState(&currElevator, floorSensorCh)
	go elevio.PollButtons(buttonCh)

	// Wait until all modules are initialized
	wg_ptr.Done()
	fmt.Println("[fsm]: initialized")
	wg_ptr.Wait()
	fmt.Println("[fsm]: starting")

	//elevatorStatusCh <- currElevator
	for {
		select {

		case buttonEvent := <-buttonCh:
			if buttonEvent.Button == elevio.BT_Cab {
				currElevator.Orders[buttonEvent.Floor][buttonEvent.Button] = true
				currElevator.Lights[buttonEvent.Floor][buttonEvent.Button] = true
				elevio.SetButtonLamp(buttonEvent.Button, buttonEvent.Floor, true)
				//fmt.Println("[fsm]: Cab order added and lights turned on")
				//fmt.Println("[fsm]: Estimated completion time: %f", EstimatedCompletionTime(currElevator, OrderEvent{buttonEvent.Floor, buttonEvent.Button, false}))
				switch currElevator.State {
				case IDLE:
					if shouldOpenDoor(currElevator) { //buttonEvent.Floor == last_floor
						setStateToDoorOpen(&currElevator, doorTimer)
						clearOrdersAtFloor(&currElevator, false)
					} else {
						updateElevatorDirection(&currElevator)
						setStateToDrive(&currElevator)
					}
				case DOOR_OPEN: // a new order -> extend timer, determine direction
					if shouldOpenDoor(currElevator) { //buttonEvent.Floor == last_floor
						setStateToDoorOpen(&currElevator, doorTimer)
						clearOrdersAtFloor(&currElevator, false)
					} else {
						updateElevatorDirection(&currElevator)
					}
				}
			} else {
				placedHallOrderCh.Send <- OrderEvent{Floor: buttonEvent.Floor, Button: buttonEvent.Button, TurnLightOn: false}
			}

		case msg, _ := <-addHallOrderCh.Recv:
			hallOrder := msg.(OrderEvent)
			currElevator.Orders[hallOrder.Floor][hallOrder.Button] = true
			enableLight := hallOrder.TurnLightOn || currElevator.Lights[hallOrder.Floor][hallOrder.Button]
			elevio.SetButtonLamp(hallOrder.Button, hallOrder.Floor, enableLight)
			currElevator.Lights[hallOrder.Floor][hallOrder.Button] = enableLight

			//fmt.Println("[fsm]: Estimated completion time: %f", EstimatedCompletionTime(currElevator, hallOrder))
			switch currElevator.State {
			case IDLE:
				if shouldOpenDoor(currElevator) { //buttonEvent.Floor == last_floor
					setStateToDoorOpen(&currElevator, doorTimer)
					clearOrdersAtFloor(&currElevator, false)
					fmt.Println("[fsm]: Door opens")
				} else {
					updateElevatorDirection(&currElevator)
					setStateToDrive(&currElevator)
				}
			case DOOR_OPEN: // a new order -> extend timer, determine direction
				if shouldOpenDoor(currElevator) { //buttonEvent.Floor == last_floor
					setStateToDoorOpen(&currElevator, doorTimer)
					clearOrdersAtFloor(&currElevator, false)
					////fmt.Println("Door keeps open")
				} else {
					updateElevatorDirection(&currElevator)
				}
			default:
				updateElevatorDirection(&currElevator)
			}

		case msg, _ := <-deleteHallOrderCh.Recv:
			hallOrder := msg.(OrderEvent)

			currElevator.Orders[hallOrder.Floor][hallOrder.Button] = false
			elevio.SetButtonLamp(hallOrder.Button, hallOrder.Floor, false)
			currElevator.Lights[hallOrder.Floor][hallOrder.Button] = false
			if currElevator.State == DOOR_OPEN {
				updateElevatorDirection(&currElevator)
				clearOrdersAtFloor(&currElevator, false)
			}

		case currElevator.Floor = <-floorSensorCh: //new floor reached -> door_open, idle, drive in other direction, continue drive
			//fmt.Println("[fsm]: New floor reached")
			elevio.SetFloorIndicator(currElevator.Floor)
			updateElevatorDirection(&currElevator)
			if shouldOpenDoor(currElevator) {
				setStateToDoorOpen(&currElevator, doorTimer)
				clearOrdersAtFloor(&currElevator, false)
			} else if currElevator.Dir == elevio.MD_Stop {
				setStateToIdle(&currElevator)
			} else {
				setStateToDrive(&currElevator)
			}

		case <-doorTimer.C:
			////fmt.Println("Door closes")
			elevio.SetDoorOpenLamp(false)
			updateElevatorDirection(&currElevator)
			if currElevator.Dir == elevio.MD_Stop {
				setStateToIdle(&currElevator)
			} else {
				setStateToDrive(&currElevator)
			}

		case msg, _ := <-updateLightsCh.Recv:
			updateLights := msg.([N_FLOORS][N_BUTTONS]bool)

			for floor := 0; floor < N_FLOORS; floor++ {
				for button := 0; button < N_BUTTONS-1; button++ { // note ignoring cab call lights
					if !(floor == N_FLOORS-1 && elevio.ButtonType(button) == elevio.BT_HallUp) &&
						!(floor == 0 && elevio.ButtonType(button) == elevio.BT_HallDown) {
						currElevator.Lights[floor][button] = updateLights[floor][button]
						elevio.SetButtonLamp(elevio.ButtonType(button), floor, currElevator.Lights[floor][button])
					}
				}
			}
		}
		var completedHallOrderSlice []OrderEvent
		for floor := 0; floor < N_FLOORS; floor++ {
			for button := 0; button < N_BUTTONS; button++ {
				if button < 2 && currElevator.CompletedOrders[floor][button] {
					completedHallOrderSlice = append(completedHallOrderSlice, OrderEvent{Floor: floor, Button: elevio.ButtonType(button), TurnLightOn: false})
				}
				currElevator.CompletedOrders[floor][button] = false
			}
		}
		if currElevator != prevElevator {
			completedHallOrderCh.Send <- completedHallOrderSlice
			elevatorStatusCh.Send <- currElevator
			prevElevator = currElevator
		}
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
