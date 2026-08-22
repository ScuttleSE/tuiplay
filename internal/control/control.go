// Package control is the external control protocol for tuiplay. It defines
// the command grammar and a small client that sends one command to a
// running tuiplay over a unix socket. The interface package runs the
// server side.
package control

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

// Command is one parsed control command. Name is the verb, for example
// "next" or "playlist". Arg holds the rest of the line, for example the
// playlist name or a seek amount.
type Command struct {
	Name string
	Arg  string
}

// Parse splits a command line into a verb and its argument. It trims outer
// whitespace. The verb is lower-cased. The argument keeps its own case.
func Parse(line string) Command {
	line = strings.TrimSpace(line)
	if line == "" {
		return Command{}
	}
	parts := strings.SplitN(line, " ", 2)
	name := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}
	return Command{Name: name, Arg: arg}
}

// Send connects to the unix socket at path, writes one command line, reads
// the one-line reply, and returns it. It uses a short timeout so a broken
// or missing server does not hang the caller.
func Send(path, line string) (string, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return "", fmt.Errorf("connect to %s: %w", path, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprintln(conn, line); err != nil {
		return "", fmt.Errorf("send: %w", err)
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	reply = strings.TrimRight(reply, "\n")
	if err != nil && reply == "" {
		return "", fmt.Errorf("read reply: %w", err)
	}
	return reply, nil
}
