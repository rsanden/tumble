SHELL:=/bin/bash

.PHONY: all clean exe test

all: exe

clean:
	cd cmd/tumble && make clean

exe: clean
	cd cmd/tumble && make exe

test:
	go test -parallel 1
	cd cmd/tumble && make test
