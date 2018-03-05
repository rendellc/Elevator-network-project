package main

import "./elevio/elevio"
import "fmt"
import "time"

const _pollRate = 20 * time.Millisecond   //import
const open_door_time_threshold = 5*time.Second

type State int

const (
    IDLE    State = 0
    DRIVE         = 1
    OPENDOOR      = 2
    UNINITIALIZED = 3
)

func PollFloorOrder(receiver chan<- int) {
  	prev := -1
  	for {
    		time.Sleep(_pollRate)
        var v int
        _, _ = fmt.Scanf("%d", &v)
    		if v != prev && v != -1 {
      			receiver <- v
    		}
    		prev = v
  	}
}

func main(){

    const numFloors = 4

    elevio.Init("localhost:15657", numFloors)

    var motor_direction elevio.MotorDirection = elevio.MD_Stop   // = MD_Stop necessary
    elevio.SetMotorDirection(motor_direction) // necessary

    drv_buttons := make(chan elevio.ButtonEvent)
    drv_floors  := make(chan int)
    //drv_obstr   := make(chan bool)
    //drv_stop    := make(chan bool)
    drv_floor_order :=make(chan int)

    var cab_call_array[numFloors] int
    var is_floor_order bool = false
    var open_door_at_current_floor bool = false
    var floor_order int
    door_timer := time.NewTimer(open_door_time_threshold)
    door_timer.Stop()
    var is_order_upstairs bool
    var is_order_downstairs bool

    // Make initialize routine to function:
    // Jalla løsning: getFloor() ble modifisert til GetFloor()
    if elevio.GetFloor() == -1{
      elevio.SetMotorDirection(elevio.MD_Down) //Uses gravity
      for elevio.GetFloor() == -1 {
        }
      elevio.SetMotorDirection(elevio.MD_Stop)
    }

    var state State = IDLE
    var current_floor int = elevio.GetFloor()


    go elevio.PollButtons(drv_buttons)
    go elevio.PollFloorSensor(drv_floors)
    //go elevio.PollObstructionSwitch(drv_obstr)
    //go elevio.PollStopButton(drv_stop)
    go PollFloorOrder(drv_floor_order)


    for {
        select {

        case floor_order = <- drv_floor_order:
            is_floor_order = true
            fmt.Printf("Order controller instructs elevator to stop at floor %+v.\n", floor_order)
            if state == IDLE {//implies motor_direction = MD.Stop
                if floor_order == current_floor {
                  state = OPENDOOR
                  fmt.Printf("The door opens\n")
                  //elevio.SetMotorDirection(elevio.MD_Stop)
                  elevio.SetDoorOpenLamp(true)
                  door_timer.Reset(open_door_time_threshold)
                  is_floor_order = false
                } else if floor_order > current_floor{
                  motor_direction = elevio.MD_Up
                  elevio.SetMotorDirection(motor_direction)
                } else {
                  motor_direction = elevio.MD_Down
                  elevio.SetMotorDirection(motor_direction)
                }
            }

        case pressed_button := <- drv_buttons:
            if pressed_button.Button == elevio.BT_Cab{
                fmt.Printf("Cab order to floor %+v registered.\n",pressed_button.Floor)
                cab_call_array[pressed_button.Floor] = 1
                elevio.SetButtonLamp(pressed_button.Button, pressed_button.Floor, true)
                if state == IDLE {//implies motor_direction = MD.Stop
                    if pressed_button.Floor == current_floor {
                      state = OPENDOOR
                      fmt.Printf("The door opens\n")
                      //elevio.SetMotorDirection(elevio.MD_Stop)
                      elevio.SetDoorOpenLamp(true)
                      door_timer.Reset(open_door_time_threshold)
                      cab_call_array[current_floor] = 0
                      elevio.SetButtonLamp(pressed_button.Button, pressed_button.Floor, false)
                    } else if pressed_button.Floor > current_floor{
                      motor_direction = elevio.MD_Up
                      elevio.SetMotorDirection(motor_direction)
                    } else {
                      motor_direction = elevio.MD_Down
                      elevio.SetMotorDirection(motor_direction)
                    }
                }
            }

        case current_floor = <- drv_floors:  //current_floor channel
            elevio.SetFloorIndicator(current_floor)
            open_door_at_current_floor = cab_call_array[current_floor]==1
            if is_floor_order == true{
                open_door_at_current_floor = open_door_at_current_floor || floor_order == current_floor
            }
            if open_door_at_current_floor {
              state = OPENDOOR
              fmt.Printf("The door opens\n")
              elevio.SetMotorDirection(elevio.MD_Stop)
              elevio.SetDoorOpenLamp(true)
              door_timer.Reset(open_door_time_threshold)
              cab_call_array[current_floor] = 0
              elevio.SetButtonLamp(elevio.BT_Cab, current_floor, false)
              if floor_order == current_floor{
                is_floor_order = false
              }
            }

          case <- door_timer.C :
              elevio.SetDoorOpenLamp(false)
              fmt.Printf("The door closes\n")
              for floor:=numFloors-1; floor > current_floor; floor-- {
                  if cab_call_array[floor]==1{
                      is_order_upstairs = true
                      //break ?
                      }
              }
              for floor:=0; floor < current_floor; floor++ {
                  if cab_call_array[floor]==1{
                    is_order_downstairs = true
                    //break ?
                    }
              }
              if is_floor_order{
                  if floor_order > current_floor {
                      is_order_upstairs = true
                  }else if floor_order < current_floor {
                      is_order_downstairs = true
                  }
              }
              if is_order_upstairs && (motor_direction == elevio.MD_Up || motor_direction == elevio.MD_Stop || !is_order_downstairs){
                state = DRIVE
                motor_direction = elevio.MD_Up
                elevio.SetMotorDirection(motor_direction)
              } else if is_order_downstairs && (motor_direction == elevio.MD_Down || motor_direction == elevio.MD_Stop || !is_order_upstairs){
                state = DRIVE
                motor_direction = elevio.MD_Down
                elevio.SetMotorDirection(motor_direction)
              } else {
                state = IDLE
                motor_direction = elevio.MD_Stop
              }
        /*
        case a := <- drv_obstr:
            fmt.Printf("%+v\n", a)
            if a {
                elevio.SetMotorDirection(elevio.MD_Stop)
            } else {
                elevio.SetMotorDirection(d)
            }
        */
        /*
        case a := <- drv_stop:
            fmt.Printf("%+v\n", a)
            for f := 0; f < numFloors; f++ {
                for b := elevio.ButtonType(0); b < 3; b++ {
                    elevio.SetButtonLamp(b, f, false)
                }
            }
        */
        }
    }
}
