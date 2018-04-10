package fsm

import (
  "./elevio/elevio"
  "fmt"
  "time"
)

// FSM variables
type State int

const (
    IDLE    State = iota
    DRIVE
    DOOR_OPEN
)

// Intern order handle variables
const N_FLOORS = 4 //import
const N_BUTTONS = 3
// Door timer variables
const door_open_time_threshold = 3.0  // time.Second
var doorTimer = time.NewTimer(door_open_time_threshold*time.Second) * time.Second
//
const pollElevatorStatusPeriod =
type Elevator struct {
    Floor int
    Dir elevio.MotorDirection
    Orders[N_FLOORS][N_BUTTONS] bool
    State State
}

// FSM functions
func initalize_state(elev *Elevator, floorSensorCh <-chan int) {
  // Add timer? After timer goes out, then drive down.
  fmt.Println("Initializing")
  elevio.SetMotorDirection(elevio.MD_Down)
  elev.Floor = <- floorSensorCh
  elev.Dir = elevio.MD_Stop
  set_state_to_idle(elev)
  elevio.SetFloorIndicator(elev.Floor)
}

func set_state_to_door_open(elev *Elevator){
  elev.State = DOOR_OPEN
  elevio.SetMotorDirection(elevio.MD_Stop)
  elevio.SetDoorOpenLamp(true)
  doorTimer.Reset(door_open_time_threshold*time.Second)
}

func set_state_to_drive(elev *Elevator){
  elev.State = DRIVE
  elevio.SetMotorDirection(elev.Dir)
}

func set_state_to_idle(elev *Elevator){
  elev.State = IDLE
  elevio.SetMotorDirection(elev.Dir)
}

func is_order_upstairs(elev Elevator) bool {
  for floor := elev.Floor+1; floor < N_FLOORS; floor++ {
    for button := 0; button < N_BUTTONS; button++ {
      if elev.Orders[floor][button] {
        return true
      }
    }
  }
  return false
}

func is_order_downstairs(elev Elevator) bool {
  for floor := 0; floor < elev.Floor; floor++ {
    for button := 0; button < N_BUTTONS; button++ {
      if elev.Orders[floor][button] {
        return true
      }
    }
  }
  return false
}

func elev_should_open_door(elev Elevator) bool{
  if elev.Dir == elevio.MD_Up{
    if elev.Orders[elev.Floor][elevio.BT_Cab] ||
    elev.Orders[elev.Floor][elevio.BT_HallUp] ||
    !is_order_upstairs(elev) && elev.Orders[elev.Floor][elevio.BT_HallDown]{
      return true
    }
  }else if elev.Dir == elevio.MD_Down{
    if elev.Orders[elev.Floor][elevio.BT_Cab] ||
    elev.Orders[elev.Floor][elevio.BT_HallDown] ||
    !is_order_downstairs(elev) && elev.Orders[elev.Floor][elevio.BT_HallUp]{
      return true
    }
  }else { //different from files online
    if elev.Orders[elev.Floor][elevio.BT_Cab] ||
    elev.Orders[elev.Floor][elevio.BT_HallUp] ||
    elev.Orders[elev.Floor][elevio.BT_HallDown] {
      return true
    }
  }
  return  false
}
// Intern order handler
func update_elevator_direction(elev *Elevator){
  if elev.Dir == elevio.MD_Up {
    if !is_order_upstairs(*elev){
      if is_order_downstairs(*elev){
        elev.Dir = elevio.MD_Down
      }else{
        elev.Dir = elevio.MD_Stop
      }
    }
  }else if elev.Dir == elevio.MD_Down {
    if !is_order_downstairs(*elev){
      if is_order_upstairs(*elev){
        elev.Dir = elevio.MD_Up
      }else{
        elev.Dir = elevio.MD_Stop
      }
    }
  }else{
    if is_order_upstairs(*elev){
      elev.Dir = elevio.MD_Up
    }else if is_order_downstairs(*elev){
      elev.Dir = elevio.MD_Down
    }
  }
}

func clear_requests_at_floor(elev *Elevator){
  if elev.Dir == elevio.MD_Up {
    elev.Orders[elev.Floor][elevio.BT_HallUp]=false
    elev.Orders[elev.Floor][elevio.BT_Cab]=false
    if !is_order_upstairs(*elev){
      elev.Orders[elev.Floor][elevio.BT_HallDown]=false
    }
  }else if elev.Dir == elevio.MD_Down {
    elev.Orders[elev.Floor][elevio.BT_HallDown]=false
    elev.Orders[elev.Floor][elevio.BT_Cab]=false
    if !is_order_downstairs(*elev){
      elev.Orders[elev.Floor][elevio.BT_HallUp]=false
    }
  }else{
    elev.Orders[elev.Floor][elevio.BT_HallUp]=false
    elev.Orders[elev.Floor][elevio.BT_HallDown]=false
    elev.Orders[elev.Floor][elevio.BT_Cab]=false
  }
}

func clear_lights_at_floor(elev Elevator){
  if elev.Dir == elevio.MD_Up {
    elevio.SetButtonLamp(elevio.BT_HallUp, elev.Floor, false)
    elevio.SetButtonLamp(elevio.BT_Cab, elev.Floor, false)
    if !is_order_upstairs(elev){
      elevio.SetButtonLamp(elevio.BT_HallDown, elev.Floor, false)
    }
  }else if elev.Dir == elevio.MD_Down {
    elevio.SetButtonLamp(elevio.BT_HallDown, elev.Floor, false)
    elevio.SetButtonLamp(elevio.BT_Cab, elev.Floor, false)
    if !is_order_downstairs(elev){
      elevio.SetButtonLamp(elevio.BT_HallUp, elev.Floor, false)
    }
  }else{
    elevio.SetButtonLamp(elevio.BT_HallUp, elev.Floor, false)
    elevio.SetButtonLamp(elevio.BT_HallDown, elev.Floor, false)
    elevio.SetButtonLamp(elevio.BT_Cab, elev.Floor, false)
  }
}

type OrderEvent struct {
  Floor int
  Button elevio.ButtonType
  TurnLightOn bool
}

func FSM(addHallOrderCh <-chan OrderEvent, deleteHallOrderCh <-chan elevio.ButtonEvent,
  placedHallOrderCh chan<- elevio.ButtonEvent, completedHallOrderCh chan<- elevio.ButtonEvent,
  elevatorStatusCh chan<- Elevator){

  fmt.Println("Lets start")
  elevio.Init("localhost:15657", N_FLOORS)
  fmt.Println("Hardware initialized")
  buttonCh := make(chan elevio.ButtonEvent)
  floorSensorCh  := make(chan int)
  var elevator Elevator

  //doorTimer.Stop()
  go elevio.PollFloorSensor(floorSensorCh)
  initalize_state(&elevator, floorSensorCh)
  go elevio.PollButtons(buttonCh)
  fmt.Println("Before for loop")

  for{
    select{
    case button_event := <- buttonCh:
      fmt.Println("Button Event")
      if button_event.Button == elevio.BT_Cab{
        fmt.Println("Button Event: Cab order")
        elevator.Orders[button_event.Floor][button_event.Button]=true
        elevio.SetButtonLamp(button_event.Button, button_event.Floor, true)
        fmt.Println("Cab order added and lights turned on")
        fmt.Println("Estimated completion time: %f", estimated_completion_time(elevator,button_event))
        switch elevator.State{
        case IDLE:
          if elev_should_open_door(elevator) { //button_event.Floor == last_floor
            set_state_to_door_open(&elevator)
            clear_requests_at_floor(&elevator)
            clear_lights_at_floor(elevator)
            fmt.Println("Door opens")
          }else{
            update_elevator_direction(&elevator)
            set_state_to_drive(&elevator)
            fmt.Println("Elevator begins to move")
          }
        case DOOR_OPEN: // a new order -> extend timer, determine direction
          if elev_should_open_door(elevator) { //button_event.Floor == last_floor
            set_state_to_door_open(&elevator)
            clear_requests_at_floor(&elevator)
            clear_lights_at_floor(elevator)
            fmt.Println("Door keeps open")
          }else{
            update_elevator_direction(&elevator)
          }
        }
      }else{
        fmt.Println("Button Event: Hall order")
        drv_hall_button_event <- button_event
      }

    case hall_order := <- addHallOrderCh:
      fmt.Println("Hall order")
      elevator.Orders[hall_order.Floor][hall_order.Button]=true
      elevio.SetButtonLamp(hall_order.Button, hall_order.Floor, hall_order.TurnLightOn)
      fmt.Println("Hall order added and lights turned on if requested")
      fmt.Println("Estimated completion time: %f", estimated_completion_time(elevator,elevio.ButtonEvent{hall_order.Floor, hall_order.Button}))
      switch elevator.State{
      case IDLE:
        if elev_should_open_door(elevator) { //button_event.Floor == last_floor
          set_state_to_door_open(&elevator)
          clear_requests_at_floor(&elevator)
          clear_lights_at_floor(elevator)
          fmt.Println("Door opens")
        }else{
          update_elevator_direction(&elevator)
          set_state_to_drive(&elevator)
          fmt.Println("Elevator begins to move")
        }
      case DOOR_OPEN: // a new order -> extend timer, determine direction
        if elev_should_open_door(elevator) { //button_event.Floor == last_floor
          set_state_to_door_open(&elevator)
          clear_requests_at_floor(&elevator)
          clear_lights_at_floor(elevator)
          fmt.Println("Door keeps open")
        }else{
          update_elevator_direction(&elevator)
        }

    case hall_order := <- deleteHallOrderCh
      fmt.Println("Delete order: Not implemented")

    case elevator.Floor = <-floorSensorCh: //new floor reached -> door_open, idle, drive in other direction, continue drive
    fmt.Println("New floor reached")
      elevio.SetFloorIndicator(elevator.Floor)
      if elev_should_open_door(elevator){
        set_state_to_door_open(&elevator)
        update_elevator_direction(&elevator)
        clear_requests_at_floor(&elevator)
        clear_lights_at_floor(elevator)
      }
    // when delete order feature added: continue driving? drive in opossite direction? go to idle

    case <- doorTimer.C :
      fmt.Println("Door closes")
      elevio.SetDoorOpenLamp(false)
      update_elevator_direction(&elevator)
      if elevator.Dir == elevio.MD_Stop {
        set_state_to_idle(&elevator)
      }else{
        set_state_to_drive(&elevator)
      }

    case <-time.After(pollElevatorStatusPeriod):
      elevatorStatusCh <- elevator
    }
  }
  fmt.Println("Lets end")
}
