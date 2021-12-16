package main

// Note: Run tests sequentially (go test -parallel 1)

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
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

func TestIntegrationTimestamp(t *testing.T) {
	setup()

	timeFormat := "2006-01-02 15:04:05.000000000"

	textLines := []string{
		"this",
		"is",
		"soon-to-be newline-delimited",
		"(but still not very interesting)",
		"text",
	}
	text := strings.Join(textLines, "\n") + "\n"

	cmd := exec.Command(
		"./tumble",
		"--logfile", "tmp/foo.log",
		"--max-log-size", "10",
		"--max-total-size", "20",
		"--time-format", timeFormat,
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

	lines := strings.Split(string(fileContent), "\n")
	for i, line := range lines {
		if i == len(textLines) {
			// This is the final empty piece at the end of the file
			if line != "" {
				t.Fatalf("Expected last line to be a empty, but it was: [%q]", line)
			}
			break
		}

		pieces := strings.Split(line, " : ")
		if len(pieces) != 2 {
			t.Fatalf("Expected this line to have 2 pieces, but it had %d: [%s]", len(pieces), line)
		}
		timestamp, msg := pieces[0], pieces[1]
		ts, err := time.Parse(timeFormat, timestamp)
		if err != nil {
			t.Fatal(err)
		}
		if time.Since(ts) > 1*time.Second {
			t.Fatalf("Longer than 1 second has passed since the encoded timestamp. Line: [%q]", line)
		}
		expectedMsg := strings.TrimSpace(textLines[i])
		if msg != expectedMsg {
			t.Fatalf("%s != %s", msg, expectedMsg)
		}
	}

	cleanup()
}
