package order_handler

import (
  "../elevio/elevio"
  "../fsm"
  "fmt"
  "time"
)

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

func EstimatedCompletionTime(elev Elevator, button_event elevio.ButtonEvent) float64{// TO DO
  duration := 0.0
  elev.Orders[button_event.Floor][button_event.Button]=true
  switch elev.State {
  case IDLE:
    fsm.update_elevator_direction(&elev)
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
    if(fsm.elev_should_open_door(elev)){
      duration += DOOR_OPEN_TIME
      fsm.clear_requests_at_floor(&elev)
      fsm.update_elevator_direction(&elev)
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
