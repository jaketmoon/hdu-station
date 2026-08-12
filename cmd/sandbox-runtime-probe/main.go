package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

type request struct {
	Version int      `json:"version"`
	Op      string   `json:"op"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

type response struct {
	Version int    `json:"version"`
	Stdout  string `json:"stdout,omitempty"`
	Stderr  string `json:"stderr,omitempty"`
	Exit    int    `json:"exit_code"`
	Error   string `json:"error,omitempty"`
}

func main() {
	socket := flag.String("socket", "", "guest runtime Unix socket")
	op := flag.String("op", "ping", "runtime operation")
	command := flag.String("command", "", "runtime command")
	timeout := flag.Duration("timeout", 10*time.Second, "request timeout")
	var args stringList
	flag.Var(&args, "arg", "runtime command argument (repeatable)")
	flag.Parse()
	if *socket == "" || (*op != "ping" && *op != "execute") || (*op == "execute" && *command == "") {
		fail(errors.New("socket, a supported operation, and an execute command when applicable are required"))
	}
	connection, err := net.DialTimeout("unix", *socket, *timeout)
	if err != nil {
		fail(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(*timeout)); err != nil {
		fail(err)
	}
	data, err := json.Marshal(request{Version: 1, Op: *op, Command: *command, Args: args})
	if err != nil {
		fail(err)
	}
	if _, err := connection.Write(append(data, '\n')); err != nil {
		fail(err)
	}
	closer, ok := connection.(interface{ CloseWrite() error })
	if !ok {
		fail(errors.New("runtime socket does not support half-close"))
	}
	if err := closer.CloseWrite(); err != nil {
		fail(err)
	}
	value, err := decodeResponse(connection)
	if err != nil {
		fail(err)
	}
	if value.Version != 1 || value.Error != "" || (*op == "execute" && value.Exit != 0) {
		fail(errors.New("runtime returned an unsuccessful response"))
	}
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		fail(err)
	}
}

type stringList []string

func (values *stringList) String() string { return "" }
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func decodeResponse(reader io.Reader) (response, error) {
	buffered := bufio.NewReader(reader)
	line, err := buffered.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return response{}, err
	}
	if len(line) == 0 {
		return response{}, errors.New("runtime returned no response")
	}
	if _, err := buffered.ReadByte(); !errors.Is(err, io.EOF) {
		if err == nil {
			return response{}, errors.New("runtime returned trailing data")
		}
		return response{}, err
	}
	var value response
	if err := json.Unmarshal(line, &value); err != nil {
		return response{}, err
	}
	return value, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
