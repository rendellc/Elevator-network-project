package main

import "./elevio/elevio"
import "fmt"
import "time"

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
var order_register_matrix[N_BUTTONS][N_FLOORS] bool //initialze as false
// Door timer variables
const door_open_time_threshold = 3.0
var door_timer = time.NewTimer(door_open_time_threshold)

type Elevator struct {
    Floor int
    Dirn elevio.MotorDirection
    Orders[N_FLOORS][N_BUTTONS] bool
    State State
}

// FSM functions
func initalize_state(elev *Elevator, drv_floor_sensor <-chan int) {
  // Add timer? After timer goes out, then drive down.
  fmt.Println("Initializing")
  elevio.SetMotorDirection(elevio.MD_Down)
  elev.Floor = <- drv_floor_sensor
  elev.Dirn = elevio.MD_Stop
  set_state_to_idle(elev)
  elevio.SetFloorIndicator(elev.Floor)
}

func set_state_to_door_open(elev *Elevator){
  elev.State = DOOR_OPEN
  elevio.SetMotorDirection(elevio.MD_Stop)
  elevio.SetDoorOpenLamp(true)
  door_timer.Reset(door_open_time_threshold*time.Second)
}

func set_state_to_drive(elev *Elevator){
  elev.State = DRIVE
  elevio.SetMotorDirection(elev.Dirn)
}

func set_state_to_idle(elev *Elevator){
  elev.State = IDLE
  elevio.SetMotorDirection(elev.Dirn)
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
  if elev.Dirn == elevio.MD_Up{
    if elev.Orders[elev.Floor][elevio.BT_Cab] ||
    elev.Orders[elev.Floor][elevio.BT_HallUp] ||
    !is_order_upstairs(elev) && elev.Orders[elev.Floor][elevio.BT_HallDown]{
      return true
    }
  }else if elev.Dirn == elevio.MD_Down{
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
  if elev.Dirn == elevio.MD_Up {
    if !is_order_upstairs(*elev){
      if is_order_downstairs(*elev){
        elev.Dirn = elevio.MD_Down
      }else{
        elev.Dirn = elevio.MD_Stop
      }
    }
  }else if elev.Dirn == elevio.MD_Down {
    if !is_order_downstairs(*elev){
      if is_order_upstairs(*elev){
        elev.Dirn = elevio.MD_Up
      }else{
        elev.Dirn = elevio.MD_Stop
      }
    }
  }else{
    if is_order_upstairs(*elev){
      elev.Dirn = elevio.MD_Up
    }else if is_order_downstairs(*elev){
      elev.Dirn = elevio.MD_Down
    }
  }
}

func clear_requests_at_floor(elev *Elevator){
  if elev.Dirn == elevio.MD_Up {
    elev.Orders[elev.Floor][elevio.BT_HallUp]=false
    elev.Orders[elev.Floor][elevio.BT_Cab]=false
    if !is_order_upstairs(*elev){
      elev.Orders[elev.Floor][elevio.BT_HallDown]=false
    }
  }else if elev.Dirn == elevio.MD_Down {
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
  if elev.Dirn == elevio.MD_Up {
    elevio.SetButtonLamp(elevio.BT_HallUp, elev.Floor, false)
    elevio.SetButtonLamp(elevio.BT_Cab, elev.Floor, false)
    if !is_order_upstairs(elev){
      elevio.SetButtonLamp(elevio.BT_HallDown, elev.Floor, false)
    }
  }else if elev.Dirn == elevio.MD_Down {
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
  AddOrder bool // =false <-> delete
  TurnLightOn bool
}

func fsm_module(drv_intern_hall_order <-chan OrderEvent, drv_hall_button_event chan<- elevio.ButtonEvent){
  fmt.Println("Lets start")
  elevio.Init("localhost:15657", N_FLOORS)
  fmt.Println("Hardware initialized")
  drv_buttons := make(chan elevio.ButtonEvent)
  drv_floor_sensor  := make(chan int)
  var elevator Elevator

  //door_timer.Stop()
  go elevio.PollFloorSensor(drv_floor_sensor)
  initalize_state(&elevator, drv_floor_sensor)
  go elevio.PollButtons(drv_buttons)
  fmt.Println("Before for loop")

  for{
    select{
    case button_event := <- drv_buttons:
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

    case hall_order := <- drv_intern_hall_order:
      fmt.Println("Hall order")
      if hall_order.AddOrder {
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
        }
      } else {
        fmt.Println("Delete order: Not implemented")
      }

    case elevator.Floor = <-drv_floor_sensor: //new floor reached -> door_open, idle, drive in other direction, continue drive
    fmt.Println("New floor reached")
      elevio.SetFloorIndicator(elevator.Floor)
      if elev_should_open_door(elevator){
        set_state_to_door_open(&elevator)
        update_elevator_direction(&elevator)
        clear_requests_at_floor(&elevator)
        clear_lights_at_floor(elevator)
      }
    // when delete order feature added: continue driving? drive in opossite direction? go to idle

    case <- door_timer.C :
      fmt.Println("Door closes")
      elevio.SetDoorOpenLamp(false)
      update_elevator_direction(&elevator)
      if elevator.Dirn == elevio.MD_Stop {
        set_state_to_idle(&elevator)
      }else{
        set_state_to_drive(&elevator)
      }
    }
  }
  fmt.Println("Lets end")
}

func simplified_order_controller(drv_intern_hall_order chan<- OrderEvent, drv_hall_button_event <-chan elevio.ButtonEvent){
  for{
    select{
      // short circuit order controller
    case button_event:= <-drv_hall_button_event:
      fmt.Println("Hall order in transit")
      drv_intern_hall_order <- OrderEvent{button_event.Floor, button_event.Button, true, true}
    }
  }
}

const TRAVEL_TIME = 2.5
const DOOR_OPEN_TIME = door_open_time_threshold

func estimated_completion_time(elev Elevator, button_event elevio.ButtonEvent) float64{// TO DO
  duration := 0.0
  elev.Orders[button_event.Floor][button_event.Button]=true
  switch elev.State {
  case IDLE:
    update_elevator_direction(&elev)
    if(elev.Dirn == elevio.MD_Stop){
      return duration
    }
  case DRIVE:
    duration += TRAVEL_TIME/2
    elev.Floor += int(elev.Dirn)
  case DOOR_OPEN:
    duration -= DOOR_OPEN_TIME
    elev.Orders[elev.Floor][elevio.BT_Cab]=true
  }
  for{
    if(elev_should_open_door(elev)){
      duration += DOOR_OPEN_TIME
      clear_requests_at_floor(&elev)
      update_elevator_direction(&elev)
      if(elev.Dirn == elevio.MD_Stop || duration > 60.0){// TO DO
        fmt.Println("Duration until completion %f", duration)
        return duration
      }
    }
    elev.Floor += int(elev.Dirn)
    duration += TRAVEL_TIME
    //fmt.Println("Duration until now %f", duration)
  }
}
/*
func order_controller(){

}
*/
func main(){
  drv_intern_hall_order := make(chan OrderEvent)
  drv_hall_button_event := make(chan elevio.ButtonEvent)

  go fsm_module(drv_intern_hall_order,drv_hall_button_event)
  go simplified_order_controller(drv_intern_hall_order,drv_hall_button_event)

  for{
  }
}
