package main

import "./elevio/elevio"
import "fmt"
import "time"

// FSM variables
type State int

const (
    IDLE    State = iota
    DRIVE
    OPEN_DOOR
)
var elevator_state = IDLE
var elevator_direction elevio.MotorDirection
var last_floor int
var move_after_door_closes bool
// Intern order handle variables
const num_floors = 4 //import
var order_register_matrix[3][num_floors] bool //initialze as false
// Door timer variables
const open_door_time_threshold = 5*time.Second
var door_timer = time.NewTimer(open_door_time_threshold)

// FSM functions
func initalize_state(drv_floor_sensor <-chan int) {
  // Add timer? After timer goes out, then drive down.
  fmt.Println("Initializing")
  elevio.SetMotorDirection(elevio.MD_Down)
  last_floor = <- drv_floor_sensor
  set_state_to_idle()
  elevio.SetFloorIndicator(last_floor)
}
func set_state_to_open_door(){
  elevator_state = OPEN_DOOR
  elevio.SetMotorDirection(elevio.MD_Stop)
  elevio.SetDoorOpenLamp(true)
  door_timer.Reset(open_door_time_threshold)
  move_after_door_closes = false
}
func set_state_to_drive(direction elevio.MotorDirection){
  elevator_state = DRIVE
  elevator_direction = direction
  elevio.SetMotorDirection(direction)
}
func set_state_to_idle(){
  elevator_state = IDLE
  elevator_direction = elevio.MD_Stop
  elevio.SetMotorDirection(elevator_direction)
}
func update_elevator_direction_after_door_closes(){
  if elevator_state==OPEN_DOOR && !move_after_door_closes {
    if is_order_upstairs(last_floor) && (elevator_direction == elevio.MD_Up ||
      elevator_direction == elevio.MD_Stop ||
      !is_order_downstairs(last_floor)){
        elevator_direction = elevio.MD_Up
        move_after_door_closes = true
    } else if is_order_downstairs(last_floor) &&
    (elevator_direction == elevio.MD_Down || elevator_direction == elevio.MD_Stop ||
      !is_order_upstairs(last_floor)){
        elevator_direction = elevio.MD_Down
        move_after_door_closes = true
      }
  }
}

// Intern order handler
func add_order(button_event elevio.ButtonEvent, turn_light_on bool){
  fmt.Println("Order added")
  order_register_matrix[button_event.Button][button_event.Floor]=true
  elevio.SetButtonLamp(button_event.Button, button_event.Floor, turn_light_on)
  fmt.Println("Order added")
}
func clear_order(button_event elevio.ButtonEvent){
  order_register_matrix[button_event.Button][button_event.Floor]=false
  elevio.SetButtonLamp(button_event.Button, button_event.Floor, false)
}
func get_order_status(button_event elevio.ButtonEvent) bool{
  return order_register_matrix[button_event.Button][button_event.Floor]
}
func is_order_upstairs(current_floor int) bool {
  for floor := current_floor+1; floor < num_floors; floor++ {
    for button := 0; button < 3; button++ {
      if order_register_matrix[button][floor] {
        return true
      }
    }
  }
  return false
}
func is_order_downstairs(current_floor int) bool {
  for floor := 0; floor < current_floor; floor++ {
    for button := 0; button < 3; button++ {
      if order_register_matrix[button][floor] {
        return true
      }
    }
  }
  return false
}

type OrderEvent struct {
  ButtonEvent elevio.ButtonEvent
  AddOrder bool
  TurnLightOn bool
}

func main(){

  fmt.Println("Lets start")
  elevio.Init("localhost:15657", num_floors)
  fmt.Println("Hardware initialized")
  drv_buttons := make(chan elevio.ButtonEvent)
  drv_floor_sensor  := make(chan int)
  drv_intern_hall_order := make(chan OrderEvent,1)
  drv_hall_button_event := make(chan elevio.ButtonEvent,1)
  door_timer.Stop()

  //var button_event elevio.ButtonEvent

  go elevio.PollFloorSensor(drv_floor_sensor)
  initalize_state(drv_floor_sensor)
  go elevio.PollButtons(drv_buttons)
  fmt.Println("Before for loop")

  for{
    select{
    case button_event := <- drv_buttons:
      fmt.Println("Button Event")
      if button_event.Button == elevio.BT_Cab{
        fmt.Println("Button Event: Cab order")
        add_order(button_event, true)
        switch elevator_state{
        case IDLE:
          if button_event.Floor == last_floor {
            set_state_to_open_door()
            clear_order(button_event)
            //update_elevator_direction_after_door_closes() //necessary?
          } else if button_event.Floor > last_floor {
            set_state_to_drive(elevio.MD_Up)
            fmt.Println("Going up")
          } else {
            set_state_to_drive(elevio.MD_Down)
            fmt.Println("Going down")
          }
        case OPEN_DOOR: // a new order -> extend timer, determine direction
          update_elevator_direction_after_door_closes()
          if button_event.Floor == last_floor {
            door_timer.Reset(open_door_time_threshold)
            clear_order(button_event)
          }
          //update_elevator_direction_after_door_closes()
        default:
          fmt.Println("Panic at intern order")
        }
      }else{
        fmt.Println("Button Event: Hall order")
        drv_hall_button_event <- button_event
      }
      // short circuit order controller
    case button_event:= <-drv_hall_button_event:
      fmt.Println("Hall order in transit")
      drv_intern_hall_order <- OrderEvent{button_event, true, true}

    case hall_order := <- drv_intern_hall_order:
      fmt.Println("Hall order")
      if hall_order.AddOrder {
        add_order(hall_order.ButtonEvent, hall_order.TurnLightOn)
        switch elevator_state{
        case IDLE:// a new order -> open_door or drive
          if hall_order.ButtonEvent.Floor == last_floor {
            set_state_to_open_door()
            clear_order(hall_order.ButtonEvent)
            //update_elevator_direction_after_door_closes() //necessary?
          }else if hall_order.ButtonEvent.Floor < last_floor {
            set_state_to_drive(elevio.MD_Down)
          }else {
            set_state_to_drive(elevio.MD_Up)
          }
        case OPEN_DOOR: // a new order -> extend timer, determine direction
          update_elevator_direction_after_door_closes()
          if hall_order.ButtonEvent.Floor == last_floor{
            if !move_after_door_closes ||
            ((elevator_direction == elevio.MD_Up && hall_order.ButtonEvent.Button == elevio.BT_HallUp) ||
            (elevator_direction == elevio.MD_Down && hall_order.ButtonEvent.Button == elevio.BT_HallDown)) {
              door_timer.Reset(open_door_time_threshold)
              clear_order(hall_order.ButtonEvent)
            }
          }
        default:
          fmt.Println("Panic at extern order")
        }
      } else {
        clear_order(hall_order.ButtonEvent)
        // update elevator_direction if move_after_door_closes==true
      }

    case last_floor = <-drv_floor_sensor: //new floor reached -> open_door, idle, drive in other direction, continue drive
    fmt.Println("New floor reached")
      elevio.SetFloorIndicator(last_floor)
      //update_elevator_direction
      if get_order_status(elevio.ButtonEvent{last_floor, elevio.BT_Cab}) ||
      (elevator_direction == elevio.MD_Up && (get_order_status(elevio.ButtonEvent{last_floor, elevio.BT_HallUp}) || (!is_order_upstairs(last_floor) &&
       get_order_status(elevio.ButtonEvent{last_floor, elevio.BT_HallDown})))) ||
      (elevator_direction == elevio.MD_Down && (get_order_status(elevio.ButtonEvent{last_floor, elevio.BT_HallDown}) || (!is_order_downstairs(last_floor) &&
       get_order_status(elevio.ButtonEvent{last_floor, elevio.BT_HallUp})))){
         set_state_to_open_door()
         update_elevator_direction_after_door_closes()
         if move_after_door_closes{
           if elevator_direction == elevio.MD_Up {
             clear_order(elevio.ButtonEvent{last_floor, elevio.BT_HallUp})
             clear_order(elevio.ButtonEvent{last_floor, elevio.BT_Cab})
           }else if elevator_direction == elevio.MD_Down {
             clear_order(elevio.ButtonEvent{last_floor, elevio.BT_HallDown})
             clear_order(elevio.ButtonEvent{last_floor, elevio.BT_Cab})
           }
         }else{
           clear_order(elevio.ButtonEvent{last_floor, elevio.BT_HallUp})
           clear_order(elevio.ButtonEvent{last_floor, elevio.BT_HallDown})
           clear_order(elevio.ButtonEvent{last_floor, elevio.BT_Cab})
           }
         }
         // when delete order feature added: continue driving? drive in opossite direction? go to idle

    case <- door_timer.C :
      fmt.Println("Door closes")
      elevio.SetDoorOpenLamp(false)
      update_elevator_direction_after_door_closes()
      if move_after_door_closes{
        set_state_to_drive(elevator_direction)
      }else{
        set_state_to_idle()
      }
    }
  }
  fmt.Println("Lets end")
}
