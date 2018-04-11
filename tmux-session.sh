#!/bin/sh
tmux new-session "elevators"
tmux a -t "elevators"
tmux split-window -v
tmux split-window -h -p 66
tmux split-window -h -p 50
