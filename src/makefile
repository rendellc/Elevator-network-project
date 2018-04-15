FLAGS=-v -race
EXEC=elevator.out

${EXEC}: $(filter-out $(EXEC),$(wildcard *))
	go build -o $@ -v -race
