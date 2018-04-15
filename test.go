package main

import "fmt"
import "./src/fsm"
import "./src/elevio"

const N_FLOORS = 4
const N_BUTTONS = 3

func main() {
	maxCompletionTime := - 0.5
	var maxElevator fsm.Elevator
	var maxOrderEvent fsm.OrderEvent

	for current_floor:=0; current_floor < 4; current_floor ++ {
	  for dir:=-1; dir < 2; dir++ {
	    for a00:=0; a00 < 2; a00++ {
	      for a01:=0; a01 < 2; a01++ {
	        for a02:=0; a02 < 2; a02++ {
	          for a10:=0; a10 < 2; a10++ {
	            for a11:=0; a11 < 2; a11++ {
	              for a12:=0; a12 < 2; a12++ {
	                for a20:=0; a20 < 2; a20++ {
	                  for a21:=0; a21 < 2; a21++ {
	                    for a22:=0; a22 < 2; a22++ {
	                      for a30:=0; a30 < 2; a30++ {
	                        for a31:=0; a31 < 2; a31++ {
	                          for a32:=0; a32 < 2; a32++ {
															for state:=0; state <3; state++ {
																var elevator fsm.Elevator
																elevator.Floor = current_floor
																elevator.Dir = elevio.MotorDirection(dir)
																ordersInt := [N_FLOORS][N_BUTTONS]int{ {a00, a01, a02}, {a10, a11, a12}, {a20, a21, a22}, {a30, a31, a32}}
																fmt.Println("ORDERSINT: \v", ordersInt)
																var orders [N_FLOORS][N_BUTTONS]bool
																for floor := 0; floor < N_FLOORS; floor++ {
		                        			for button := 0; button < N_BUTTONS; button++ {
																		if ordersInt[floor][button] == 1 {
																			orders[floor][button] = true
																		}
		                              }
		                            }
																elevator.Orders = orders
																elevator.State = fsm.State(state)
																for floorOrder := 0; floorOrder < N_FLOORS; floorOrder++ {
																	for buttonOrder := 0; buttonOrder < N_BUTTONS; buttonOrder++ {
																		orderEvent := fsm.OrderEvent{Floor: floorOrder, Button: elevio.ButtonType(buttonOrder)}
																		//fmt.Println("Elevator: \v", elevator)
																		//fmt.Println("OrderEvent: \v", orderEvent)
																		if !(current_floor == 0 && dir == -1 && state == 1) &&
																			 !(current_floor == 3 && dir == 1 && state == 1) &&
																			 !(floorOrder == 0 && buttonOrder == 1) &&
																			 !(floorOrder == 3 && buttonOrder == 0) &&
																			 !(state == 1 && dir == 1 && !fsm.IsOrderAbove(elevator)) &&
																			 !(state ==1 && dir == -1 && !fsm.IsOrderBelow(elevator)) &&
																			 !(dir == 0 && state == 1){
																		 	completionTime := fsm.EstimatedCompletionTime(elevator, orderEvent)
																			//fmt.Println("CompletionTime for: ")
																			//fmt.Println("Elevator: \v", elevator)
																			//fmt.Println("and order: \v", orderEvent)
																				if completionTime > maxCompletionTime {
																				maxCompletionTime = completionTime
																				maxElevator = elevator
																				maxOrderEvent = orderEvent
																				//fmt.Println("Max completionTime: \v", maxCompletionTime)
																				//fmt.Println("Elevator: \v", maxElevator)
																				//fmt.Println("and order: \v", maxOrderEvent)
																			}
																		}
																	}
																}
															}
	                          }
	                        }
	                      }
	                    }
	                  }
	                }
	              }
	            }
	          }
	        }
	      }
	    }
	  }
	}
	fmt.Println("Max completionTime: \v", maxCompletionTime)
	fmt.Println("Elevator: \v", maxElevator)
	fmt.Println("and order: \v", maxOrderEvent)
}
