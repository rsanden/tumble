package main

// Note: Run tests sequentially (go test -parallel 1)

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func setup() {
	os.RemoveAll("tmp")
	os.Mkdir("tmp", 0755)
}

func teardown() {
	os.RemoveAll("tmp")
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

	teardown()
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

	teardown()
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

	teardown()
}

func TestIntegrationTeeText(t *testing.T) {
	setup()

	textLines := []string{
		"we",
		"like",
		"testing-",
		"yes we do.",
		"we",
		"like",
		"testing;",
		"how about you?",
	}
	text := strings.Join(textLines, "\n") + "\n"

	cmd := exec.Command(
		"./tumble",
		"--logfile", "tmp/foo.log",
		"--max-log-size", "10",
		"--max-total-size", "20",
		"--tee-stdout",
		"--tee-stderr",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(text)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}
	fileContent, err := ioutil.ReadFile("tmp/foo.log")
	if err != nil {
		t.Fatal(err)
	}
	if string(fileContent) != text {
		t.Fatalf("content %q != %q", string(fileContent), text)
	}
	if stdout.String() != text {
		t.Fatalf("stdout %q != %q", stdout.String(), text)
	}
	if stderr.String() != text {
		t.Fatalf("stderr %q != %q", stderr.String(), text)
	}

	teardown()
}

func TestIntegrationTeeBinary(t *testing.T) {
	setup()

	data := []byte{0x00, 0x11, 0x22, 0x33, 0xde, 0xca, 0x00, 0x11, 0x22, 0x33}

	cmd := exec.Command(
		"./tumble",
		"--logfile", "tmp/foo.log",
		"--max-log-size", "10",
		"--max-total-size", "20",
		"--binary",
		"--tee-stdout",
		"--tee-stderr",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}
	fileContent, err := ioutil.ReadFile("tmp/foo.log")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(fileContent, data) != 0 {
		t.Fatalf("content %q != %q", fileContent, data)
	}
	if bytes.Compare(stdout.Bytes(), data) != 0 {
		t.Fatalf("stdout %q != %q", stdout.Bytes(), data)
	}
	if bytes.Compare(stderr.Bytes(), data) != 0 {
		t.Fatalf("stderr %q != %q", stderr.Bytes(), data)
	}

	teardown()
}

func TestIntegrationContinuityText(t *testing.T) {
	setup()

	cmd := exec.Command(
		"./tumble",
		"--logfile", "tmp/foo.log",
		"--max-log-size", "1",
		"--max-total-size", "10",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Each line will be 16 bytes long and every 65536 lines will be 1 MB (1024 * 1024 / 16).
	// We must not rotate faster than once per second in order to avoid clobbering an existing archive.
	// The following code takes this into consideration and sleeps 1 second where necessary.
	writeLineNumber := 1

	// Write 0.5 MB -- no rotation -- log 0.5 MB, wrote 0.5 MB -- (0 archives)
	for i := 0; i < 32768; i++ {
		msg := fmt.Sprintf("Line %010d\n", writeLineNumber)
		_, err = stdin.Write([]byte(msg))
		if err != nil {
			t.Fatal(err)
		}
		writeLineNumber += 1
	}

	time.Sleep(1 * time.Second)

	// Write 0.75 MB -- rotation -- log 0.25 MB, wrote 1.25 MB -- (1 archives)
	for i := 0; i < 49152; i++ {
		msg := fmt.Sprintf("Line %010d\n", writeLineNumber)
		_, err = stdin.Write([]byte(msg))
		if err != nil {
			t.Fatal(err)
		}
		writeLineNumber += 1
	}

	time.Sleep(1 * time.Second)

	// Write 1.0 MB -- rotation -- log 0.25 MB, wrote 2.25 MB -- (2 archives)
	for i := 0; i < 65536; i++ {
		msg := fmt.Sprintf("Line %010d\n", writeLineNumber)
		_, err = stdin.Write([]byte(msg))
		if err != nil {
			t.Fatal(err)
		}
		writeLineNumber += 1
	}

	time.Sleep(1 * time.Second)

	// Write 0.5 MB -- no rotation -- log 0.75 MB, wrote 2.75 MB -- (2 archives)
	for i := 0; i < 32768; i++ {
		msg := fmt.Sprintf("Line %010d\n", writeLineNumber)
		_, err = stdin.Write([]byte(msg))
		if err != nil {
			t.Fatal(err)
		}
		writeLineNumber += 1
	}

	// Write 0.75 MB -- rotation -- log 0.5 MB, wrote 3.5 MB -- (3 archives)
	for i := 0; i < 49152; i++ {
		msg := fmt.Sprintf("Line %010d\n", writeLineNumber)
		_, err = stdin.Write([]byte(msg))
		if err != nil {
			t.Fatal(err)
		}
		writeLineNumber += 1
	}

	stdin.Close()

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

	// Check the resulting files. (They will be processed in sorted order automatically.)
	files, err := os.ReadDir("tmp")
	if err != nil {
		t.Fatal(err)
	}

	numEmpty := 0
	readLineNumber := 1
	for _, f := range files {
		var content string
		if strings.HasSuffix(f.Name(), ".gz") {
			gzFile, err := os.Open("tmp/" + f.Name())
			if err != nil {
				t.Fatal(err)
			}
			defer gzFile.Close()
			gzreader, err := gzip.NewReader(gzFile)
			if err != nil {
				t.Fatal(err)
			}
			output, err := ioutil.ReadAll(gzreader)
			if err != nil {
				t.Fatal(err)
			}
			content = string(output)
		} else {
			output, err := ioutil.ReadFile("tmp/" + f.Name())
			if err != nil {
				t.Fatal(err)
			}
			content = string(output)
		}
		for _, line := range strings.Split(content, "\n") {
			if line == "" {
				numEmpty += 1
				continue
			}
			expected := fmt.Sprintf("Line %010d", readLineNumber)
			if line != expected {
				t.Fatalf("Expected [%s] but got [%s]", expected, line)
			}
			readLineNumber += 1
		}
	}

	// We expect numEmpty to be 4 (one for each trailing \n in the file).
	if numEmpty != 4 {
		t.Fatalf("Expected numEmpty to be 4, but it was %d", numEmpty)
	}

	// Finally, we expect both writeLineNumber and readLineNumber to be
	// (32768 + 49152 + 65536 + 32768 + 49152 + 1) = 229377
	if writeLineNumber != 229377 {
		t.Fatalf("Expected writeLineNumber to be 229377, but it was %d", writeLineNumber)
	}
	if readLineNumber != 229377 {
		t.Fatalf("Expected readLineNumber to be 229377, but it was %d", readLineNumber)
	}

	teardown()
}

func TestIntegrationContinuityBinary(t *testing.T) {
	setup()

	cmd := exec.Command(
		"./tumble",
		"--logfile", "tmp/foo.log",
		"--max-log-size", "1",
		"--max-total-size", "10",
		"--binary",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Produce a giant block of binary data. It will get written in chunks, rotated,
	// and later read back in and compared to ensure nothing was broken by the rotation boundaries.
	var dataBuf bytes.Buffer
	for i := 0; i < 1592603; i++ {
		if i%3 == 0 {
			continue
		}
		err := binary.Write(&dataBuf, binary.LittleEndian, uint32(i))
		if err != nil {
			t.Fatal(err)
		}
	}
	data := dataBuf.Bytes()

	// Each line will be 19 bytes long and we will rotate after 55189 lines.
	// It is intentional that these chunks do not fit perfectly inside of the read buffer.
	// We must not rotate faster than once per second in order to avoid clobbering an existing archive.
	// The following code takes this into consideration and sleeps 1 second where necessary.

	count := 0
	chunk := make([]byte, 19)
	checkpoints := make([]bool, 5)
	for {
		n, err := dataBuf.Read(chunk)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		n, err = stdin.Write(chunk[:n])
		if err != nil {
			t.Fatal(err)
		}
		count += n
		if count == 524286 {
			checkpoints[0] = true
			time.Sleep(1 * time.Second)
		}
		if count == 524286+786429 {
			checkpoints[1] = true
			time.Sleep(1 * time.Second)
		}
		if count == 524286+786429+1048572 {
			checkpoints[2] = true
			time.Sleep(1 * time.Second)
		}
		if count == 524286+786429+1048572+524286 {
			checkpoints[3] = true
			time.Sleep(1 * time.Second)
		}
		if count == 524286+786429+1048572+524286+786429 {
			checkpoints[4] = true
			time.Sleep(1 * time.Second)
		}
		if count == 524286+786429+1048572+524286+786429+786429 {
			panic("unreachable")
		}
	}
	for i, checkpoint := range checkpoints {
		if !checkpoint {
			t.Fatalf("Checkpoint %d was never reached", i)
		}
	}

	stdin.Close()

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

	// Check the resulting files. (They will be processed in sorted order automatically.)
	files, err := os.ReadDir("tmp")
	if err != nil {
		t.Fatal(err)
	}

	var fileContent []byte
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".gz") {
			gzFile, err := os.Open("tmp/" + f.Name())
			if err != nil {
				t.Fatal(err)
			}
			defer gzFile.Close()
			gzreader, err := gzip.NewReader(gzFile)
			if err != nil {
				t.Fatal(err)
			}
			output, err := ioutil.ReadAll(gzreader)
			if err != nil {
				t.Fatal(err)
			}
			fileContent = append(fileContent, output...)
		} else {
			output, err := ioutil.ReadFile("tmp/" + f.Name())
			if err != nil {
				t.Fatal(err)
			}
			fileContent = append(fileContent, output...)
		}
	}
	if bytes.Compare(fileContent, data) != 0 {
		t.Fatal("fileContent mismatch")
	}

	teardown()
}

func TestIntegrationClose(t *testing.T) {
	setup()

	cmd := exec.Command(
		"./tumble",
		"--logfile", "tmp/foo.log",
		"--max-log-size", "2",
		"--max-total-size", "10",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Write 1.95 MB -- no rotation yet
	for i := 0; i < 127795; i++ {
		msg := fmt.Sprintf("---%09d---\n", i)
		_, err = stdin.Write([]byte(msg))
		if err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir("tmp")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected 1 file but instead found %d", len(files))
	}
	if files[0].Name() != "foo.log" {
		t.Fatalf("Expected foo.log but instead found %s", files[0].Name())
	}

	// Allow any ongoing write to finish. Get ready for rotation.
	time.Sleep(200 * time.Millisecond)

	fi, err := os.Stat("tmp/foo.log")
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 127795*16 {
		t.Fatalf("Expected foo.log to have size 2044720 but instead found %d", fi.Size())
	}

	// Next, force a rotation+compression and then close immediately.
	// The program should still complete the compression for exiting.
	for i := 0; i < 6554; i++ {
		msg := fmt.Sprintf("+++%09d+++\n", i)
		_, err = stdin.Write([]byte(msg))
		if err != nil {
			t.Fatal(err)
		}
	}

	stdin.Close()

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

	// Now there should two files only- one current log and one compressed backup
	files, err = os.ReadDir("tmp")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("Expected 2 files but instead found %d", len(files))
	}
	if !strings.HasSuffix(files[0].Name(), ".log.gz") {
		t.Fatalf("Expected a compressed log but instead found %s", files[0].Name())
	}
	if files[1].Name() != "foo.log" {
		t.Fatalf("Expected foo.log but instead found %s", files[1].Name())
	}
}
