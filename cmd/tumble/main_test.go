package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func cleanup() {
	os.RemoveAll("tmp")
}

func setup() {
	cleanup()
	os.Mkdir("tmp", 0755)
}

func TestIntegrationTextMode(t *testing.T) {
	setup()

	text := "this\nis\nnewline-delimited\nbut not very incredible\ntext\n"

	cmd := exec.Command(
		"./tumble",
		"--logfile", "tmp/foo.log",
		"--max-log-size", "10",
		"--max-total-size", "20",
	)
	cmd.Stdin = strings.NewReader(text)
	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	fileContent, err := ioutil.ReadFile("tmp/foo.log")
	if err != nil {
		t.Fatal(err)
	}
	if string(fileContent) != text {
		t.Fatalf("%q != %q", string(fileContent), text)
	}

	cleanup()
}

func TestIntegrationBinaryMode(t *testing.T) {
	setup()

	data := []byte{0x00, 0x11, 0x22, 0x33, 0xde, 0xca, 0x00, 0x11, 0x22, 0x33}

	cmd := exec.Command(
		"./tumble",
		"--logfile", "tmp/foo.log",
		"--max-log-size", "10",
		"--max-total-size", "20",
		"--binary",
	)
	cmd.Stdin = bytes.NewReader(data)
	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	fileContent, err := ioutil.ReadFile("tmp/foo.log")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(fileContent, data) != 0 {
		t.Fatalf("%q != %q", fileContent, data)
	}

	cleanup()
}
