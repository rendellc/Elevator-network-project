# Technical Specification

### 1. Basic Definitions
1. An order is placed when an order button is pressed on the elevator panel inside an elevator, or the elevator panel outside the elevator.
1. A cab call (or cab order) is any order that is placed from a panel inside an elevator.
1. A hall call (or hall order) is any order that is placed from a panel outside an elevator.
1. The participating elevators is a set of elevators that remain constant over a test case. This means that even though an elevator is temporarily disabled or non functioning it is still a participating elevator. Furthermore, any elevator that may influence the outcome of the test is a participating elevator.
1. The number of participating elevators is refered to as `n`.
1. Everyone of the `n` participating elevators have the exact same floors.
1. The number of floors any of elevator see is refered to as `m`.
1. At the start of a test case no orders are taken.
1. An order is taken when a participating elevator changes the light corresponding to the order from off to on. If an order is already taken and this happens, it changes nothing.
1. An order is served when a participating elevator open it's doors at the corresponding floor and the corresponding light is in (or is turned to) off state for this particular elevator. We say that this particular elevator has served the order.

### 2. General Intentions
1. This specification describe software for controlling `n` elevators working in parallel across `m` floors.
1. When a hall order is taken, it must be served within reasonable time.
1. When a cab order is taken, it must be served within reasonable time. The only elevator able to serve such order is the elevator corresponding to the panel where the order was placed.
1. It is not reasonable to expect the order to be served as long as the only participating elevator able (as described in 2.3) to serve it is non functional.
1. Multiple elevators must be more efficient than one in cases where this is reasonable to expect.
1. The elevator system should be efficient and avoid unnecessary actions.
1. The elevators should have a sense of direction, more specifically, the elevators must avoid serving hall-up and hall-down orders on the same floor at the same time.
1. A placed order can be disgarded indefenately many times if there are no way to assure redundancy.
1. A placed order can be disgarded as long as the order will be accepted within a reasonable amount of placement attempts spread over a reasonable amount of time.
1. The door must never be open while moving.
1. The door must only be open when the elevator is at a floor.
1. When the door is opened, it should remain open for a reasonable amount of time.
1. The panel lights must be synchronized between elevators whenever this is possible.

### 3. Fault Tolerance
1. Assume that errors will happen (both expected and unexpected).
1. Assume that errors will be resolved but not within a definite amount of time.
1. Assume that multiple errors will not happen simultaneous. More specifically, assume that you have sufficient time to do necessary preparation before the next error will occur as long as you detect the first error and recover from it within reasonable time.
1. Even though errors will not happen simultaneously they can be "active" simultanously. For instance when the second error happen before the first one is resolved.
1. Loss of packets in UDP is not regarded as an error. UDP is in nature unreliable and can rightfully drop arbitrary packets. As a consequence of this, you may enounter an error at the same time as UDP packets are dropped.

### 4. Unspecified Behaviour
1. What happens after the stop button is pressed is intentionaly unspecified.
1. What happens after the obstruction button switch is turned on is intentionaly unspecified.


