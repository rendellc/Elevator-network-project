# TTK4145 - Real-time Programming Project

Code for a distributed elevator system. 
Project for the NTNU course Real-time Programming TTK4145 (spring 2018).

## Features
* Order redundancy
* UDP broadcasting based communication protocol with reliable transmission mechanisms in application layer.
* Automatic order transfers
* Support for 255 networked cooperating elevators
* Master-Slave relationship on per-order basis.
* Order distribution minimizes overall completion time


## Quickstart
Make sure to have all prerequisites listed before getting started. 
Clone this repo with all submodules:
```
git clone --recurse-submodules https://github.com/rendellc/ttk4145-project.git 
cd ttk4545-project
```
To run simulation mode, go to the Simulator-V2 directory, build it as described there. Start simulator with any port except 20010. For instance
```
./SimElevatorServer --port 20011
```
Connect to the elevator software with
```
./elevator.out -id=1 -addr="localhost:20011"
```
## Flags
* `-id=n` number in range 0-255 (required)
* `[-addr="IP-address:port"]` elevator is running on. Defaults to "localhost:15657" when unspecified
* `[-bport=m]` Port which all elevators will broadcast on. Defaults to 20010 when unspecified

## Prerequisites
To build from source:
* [Golang v1.8](https://golang.org/) - to build from source, golang v1.8 or above is needed

## Build
Be sure that go version 1.8 or higher is installed with ``go version``. Build with make from the src directory.
```
make
```
or
```
go build -o elevator.out
```

## Coding convensions
1. Channels names are given postifix describing either what module they write to, or what module they read from. For instance ``<some_content_describing_name>_fsmCh``. This would either write to fsm module or read from, which should be clear from the context. 
2. Channels are either read or write in a given submodule. No two-way channels. When this can't be enforced by compiler (for instance when using a custom channel-library), this should still be followed in the code. 

## 3rd party libraries
The following libraries are used without modifications
* [go-nonblockingchannels](https://github.com/hectane/go-nonblockingchan) - Small library for nonblocking (writes) channels. 
* [elevio](https://github.com/TTK4145/driver-go) - Go drivers for low level hardware interface to elevators
* [Simulator mkII](https://github.com/TTK4145/Simulator-v2/) - Accurate elevator simulator 
* [Elevator Server](https://github.com/TTK4145/elevator-server) - Server for hardware connection in the Real-time lab

The following libraries have been modified
* [network-go](https://github.com/TTK4145/Network-go) - Low level network drivers 

## License
This project is licensed under the MIT License - see the [LICENSE.md](LICENSE.md) file for details

