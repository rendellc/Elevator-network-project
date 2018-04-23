package fsm

import (
	"../elevio"
	"../go-nonblockingchan"
	"log"
	"os"
	"sync"
	"time"
)

var Info *log.Logger

const N_FLOORS = 4
const N_BUTTONS = 3
const DOOR_OPEN_TIME = 3.0

type State int

const (
	ST_Idle State = iota
	ST_Moving
	ST_DoorOpen
)

type Elevator struct {
	Floor           int
	Dir             elevio.MotorDirection
	Orders          [N_FLOORS][N_BUTTONS]bool
	CompletedOrders [N_FLOORS][N_BUTTONS]bool // orders completed in one iteration
	Lights          [N_FLOORS][N_BUTTONS]bool
	State           State
}

type OrderEvent struct {
	Floor       int
	Button      elevio.ButtonType
	TurnLightOn bool
}

func FSM(elevServerAddr string,
	/* Read channels */
	addOrder_orderhandlerCh *nbc.NonBlockingChan,
	deleteHallOrder_orderhandlerCh *nbc.NonBlockingChan,
	updateLights_orderhandlerCh *nbc.NonBlockingChan,
	/* Write channels */
	placedOrder_orderhandlerCh *nbc.NonBlockingChan,
	completedHallOrders_orderhandlerCh *nbc.NonBlockingChan,
	elevatorStatus_orderhandlerCh *nbc.NonBlockingChan,
	/* Sync */
	wg_ptr *sync.WaitGroup) {

	Info = log.New(os.Stdout, "[fsm]: ", 0)

	var elevator Elevator
	var doorTimer = time.NewTimer(DOOR_OPEN_TIME * time.Second)
	doorTimer.Stop()
	buttonCh := make(chan elevio.ButtonEvent)
	floorSensorCh := make(chan int)

	elevio.Init(elevServerAddr, N_FLOORS)
	go elevio.PollFloorSensor(floorSensorCh)
	initializeState(&elevator, floorSensorCh)
	go elevio.PollButtons(buttonCh)

	// Wait until all modules are initialized
	wg_ptr.Done()
	Info.Println("initialized")
	wg_ptr.Wait()
	Info.Println("starting")

	for {
		elevatorStatus_orderhandlerCh.Send <-elevator
		select {
		case buttonEvent := <-buttonCh:
			orderEvent := OrderEvent{Floor: buttonEvent.Floor, Button: buttonEvent.Button}
			if buttonEvent.Button == elevio.BT_Cab {
				orderEvent.TurnLightOn = true
				fsmOnAddedOrder(&elevator, doorTimer, orderEvent)
			} else {
				placedOrder_orderhandlerCh.Send <-orderEvent
			}

		case msg, _ := <-addOrder_orderhandlerCh.Recv:
			order := msg.(OrderEvent)
			fsmOnAddedOrder(&elevator, doorTimer, order)

		case msg, _ := <-deleteHallOrder_orderhandlerCh.Recv:
			hallOrder := msg.(OrderEvent)
			clearOrder(&elevator, hallOrder.Button, true)
			// hallOrder was not completed by this elevator. Hence,
			elevator.CompletedOrders[hallOrder.Floor][hallOrder.Button] = false
			Info.Printf("deleteHallOrder %+v\n", hallOrder)
			if elevator.State == ST_DoorOpen {
				updateElevatorDirection(&elevator)
				clearOrdersAtFloor(&elevator, true)
			}

		case elevator.Floor = <-floorSensorCh:
			elevio.SetFloorIndicator(elevator.Floor)
			if shouldOpenDoor(elevator) {
				clearOrdersAtFloor(&elevator, true)
				setStateToDoorOpen(&elevator, doorTimer)
				updateElevatorDirection(&elevator)
			} else {
				updateElevatorDirection(&elevator)
				if elevator.Dir == elevio.MD_Stop {
					setStateToIdle(&elevator)
				} else { // elevator can change direction. Relevant when orders are deleted
					setStateToDrive(&elevator)
				}
			}

		case <-doorTimer.C:
			elevio.SetDoorOpenLamp(false)
			updateElevatorDirection(&elevator)
			if elevator.Dir == elevio.MD_Stop {
				setStateToIdle(&elevator)
			} else {
				setStateToDrive(&elevator)
			}

		case msg, _ := <-updateLights_orderhandlerCh.Recv:
			updateLights := msg.([N_FLOORS][N_BUTTONS]bool)
			for floor := 0; floor < N_FLOORS; floor++ {
				for button := 0; button < N_BUTTONS; button++ {
					if elevio.ButtonType(button) != elevio.BT_Cab &&
						!(floor == N_FLOORS-1 && elevio.ButtonType(button) == elevio.BT_HallUp) &&
						!(floor == 0 && elevio.ButtonType(button) == elevio.BT_HallDown) {
						elevator.Lights[floor][button] = updateLights[floor][button]
						elevio.SetButtonLamp(elevio.ButtonType(button), floor, elevator.Lights[floor][button])
					}
				}
			}
		}

		var completedHallOrders []OrderEvent
		for floor := 0; floor < N_FLOORS; floor++ {
			for button := 0; button < N_BUTTONS; button++ {
				if elevio.ButtonType(button) != elevio.BT_Cab &&
					elevator.CompletedOrders[floor][button] {
					completedOrder := OrderEvent{Floor: floor, Button: elevio.ButtonType(button)}
					completedHallOrders = append(completedHallOrders, completedOrder)
				}
				elevator.CompletedOrders[floor][button] = false
			}
		}
		if len(completedHallOrders) > 0 {
			completedHallOrders_orderhandlerCh.Send <-completedHallOrders
			Info.Printf("completedHallOrders: %v", completedHallOrders)
		}
	}
}

func initializeState(elev *Elevator, floorSensorCh <-chan int) {
	elevio.SetStopLamp(false)
	elevio.SetDoorOpenLamp(false)
	for floor := 0; floor < N_FLOORS; floor++ {
		for button := 0; button < N_BUTTONS; button++ {
			elevio.SetButtonLamp(elevio.ButtonType(button), floor, false)
		}
	}
	elevio.SetMotorDirection(elevio.MD_Down)
	elev.Floor = <-floorSensorCh
	elev.Dir = elevio.MD_Stop
	setStateToIdle(elev)
	elevio.SetFloorIndicator(elev.Floor)
}

func fsmOnAddedOrder(elev *Elevator, doorTimer *time.Timer, order OrderEvent) {
	elev.Orders[order.Floor][order.Button] = true
	orderLightStatus := elev.Lights[order.Floor][order.Button]
	orderLightStatus = orderLightStatus || order.TurnLightOn
	elev.Lights[order.Floor][order.Button] = orderLightStatus
	elevio.SetButtonLamp(order.Button, order.Floor, orderLightStatus)
	switch elev.State {
	case ST_Idle:
		if shouldOpenDoor(*elev) {
			setStateToDoorOpen(elev, doorTimer)
			clearOrdersAtFloor(elev, true)
		} else {
			updateElevatorDirection(elev)
			setStateToDrive(elev)
		}
	case ST_DoorOpen:
		if shouldOpenDoor(*elev) {
			setStateToDoorOpen(elev, doorTimer)
			clearOrdersAtFloor(elev, true)
		} else {
			updateElevatorDirection(elev)
		}
	}
}

func setStateToDoorOpen(elev *Elevator, doorTimer *time.Timer) {
	elev.State = ST_DoorOpen
	elevio.SetMotorDirection(elevio.MD_Stop)
	elevio.SetDoorOpenLamp(true)
	doorTimer.Reset(DOOR_OPEN_TIME * time.Second)
}

func setStateToDrive(elev *Elevator) {
	elev.State = ST_Moving
	elevio.SetMotorDirection(elev.Dir)
}

func setStateToIdle(elev *Elevator) {
	elev.State = ST_Idle
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
	shouldOpenDoor := false
	switch elev.Dir {
	case elevio.MD_Up:
		shouldOpenDoor = elev.Orders[elev.Floor][elevio.BT_Cab] ||
			elev.Orders[elev.Floor][elevio.BT_HallUp] ||
			(!isOrderAbove(elev) && elev.Orders[elev.Floor][elevio.BT_HallDown])

	case elevio.MD_Down:
		shouldOpenDoor = elev.Orders[elev.Floor][elevio.BT_Cab] ||
			elev.Orders[elev.Floor][elevio.BT_HallDown] ||
			(!isOrderBelow(elev) && elev.Orders[elev.Floor][elevio.BT_HallUp])

	case elevio.MD_Stop:
		shouldOpenDoor = elev.Orders[elev.Floor][elevio.BT_Cab] ||
			elev.Orders[elev.Floor][elevio.BT_HallUp] ||
			elev.Orders[elev.Floor][elevio.BT_HallDown]
	}
	return shouldOpenDoor
}

func updateElevatorDirection(elev *Elevator) {
	switch elev.Dir {
	case elevio.MD_Up:
		if !isOrderAbove(*elev) {
			if isOrderBelow(*elev) {
				elev.Dir = elevio.MD_Down
			} else {
				elev.Dir = elevio.MD_Stop
			}
		}
	case elevio.MD_Down:
		if !isOrderBelow(*elev) {
			if isOrderAbove(*elev) {
				elev.Dir = elevio.MD_Up
			} else {
				elev.Dir = elevio.MD_Stop
			}
		}
	case elevio.MD_Stop:
		if isOrderAbove(*elev) {
			elev.Dir = elevio.MD_Up
		} else if isOrderBelow(*elev) {
			elev.Dir = elevio.MD_Down
		}
	}
}

func clearOrder(elev *Elevator, buttonType elevio.ButtonType, hasHardwareAccess bool) {
	if elev.Orders[elev.Floor][buttonType] {
		elev.Orders[elev.Floor][buttonType] = false
		elev.CompletedOrders[elev.Floor][buttonType] = true
		elev.Lights[elev.Floor][buttonType] = false
		if hasHardwareAccess {
			elevio.SetButtonLamp(buttonType, elev.Floor, false)
		}
	}
}

func clearOrdersAtFloor(elev *Elevator, canClearLight bool) {
	switch elev.Dir {
	case elevio.MD_Up:
		clearOrder(elev, elevio.BT_HallUp, canClearLight)
		clearOrder(elev, elevio.BT_Cab, canClearLight)
		if !isOrderAbove(*elev) {
			clearOrder(elev, elevio.BT_HallDown, canClearLight)
		}
	case elevio.MD_Down:
		clearOrder(elev, elevio.BT_HallDown, canClearLight)
		clearOrder(elev, elevio.BT_Cab, canClearLight)
		if !isOrderBelow(*elev) {
			clearOrder(elev, elevio.BT_HallUp, canClearLight)
		}
	case elevio.MD_Stop:
		clearOrder(elev, elevio.BT_HallUp, canClearLight)
		clearOrder(elev, elevio.BT_HallDown, canClearLight)
		clearOrder(elev, elevio.BT_Cab, canClearLight)
	}
}

func EstimatedCompletionTime(elev Elevator, orderEvent OrderEvent) float64 {
	const TRAVEL_TIME = 2.5
	duration := 0.0
	elev.Orders[orderEvent.Floor][orderEvent.Button] = true
	switch elev.State {
	case ST_Idle:
		updateElevatorDirection(&elev)
		if elev.Dir == elevio.MD_Stop {
			return duration
		}
	case ST_Moving:
		duration += TRAVEL_TIME / 2
		elev.Floor += int(elev.Dir)
	case ST_DoorOpen:
		duration -= DOOR_OPEN_TIME / 2
		updateElevatorDirection(&elev)
		if elev.Dir == elevio.MD_Stop {
			return duration
		}
	}
	for {
		if shouldOpenDoor(elev) {
			duration += DOOR_OPEN_TIME
			clearOrdersAtFloor(&elev, false)
			updateElevatorDirection(&elev)
			if elev.Dir == elevio.MD_Stop {
				return duration
			}
		}
		elev.Floor += int(elev.Dir)
		duration += TRAVEL_TIME
	}
}
