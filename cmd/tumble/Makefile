SHELL:=/bin/bash

.PHONY: all clean exe test

all: exe

clean:
	rm -rf tmp
	rm -f tumble

exe: clean
	go build
	strip tumble

test: exe
	go test -parallel 1
