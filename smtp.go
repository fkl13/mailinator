package main

import (
	"context"
	"io"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

type sessionState int

const (
	stateNotYetGreeted sessionState = iota
	stateGreeted
	stateMailFromSet
	stateRcptSet
	stateReadyForData
)

const maxMessageSize = 25 * 1024 * 1024

type smtpServer struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
	wg    sync.WaitGroup
}

func newServer() *smtpServer {
	return &smtpServer{
		conns: make(map[net.Conn]struct{}),
	}
}

func (s *smtpServer) trackConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.conns[conn] = struct{}{}
}

func (s *smtpServer) untrackConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.conns, conn)
}

func (s *smtpServer) closeAllConns() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.conns {
		conn.Close()
	}
}

func (s *smtpServer) serveSMTP(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			ln.Close()
		case <-done:
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}

		s.trackConn(conn)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.untrackConn(conn)

			handleConn(conn)
		}()

	}

	waitCh := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// all handlers finished cleanly
	case <-time.After(5 * time.Second):
		s.closeAllConns()
		<-waitCh // handleConn should return promptly once its conn is closed
	}

	return ctx.Err()
}

type session struct {
	conn       *textproto.Conn
	state      sessionState
	helo       string   // hostname client gave in EHLO
	sender     string   // MAIL FROM value
	recipients []string // RCPT TO values
	message    string
	err        error
	maxSize    int64
	quit       bool
}

func newSession(conn *textproto.Conn) *session {
	return &session{
		conn:    conn,
		state:   stateNotYetGreeted,
		maxSize: maxMessageSize,
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	tp := textproto.NewConn(conn)
	session := newSession(tp)
	if err := session.conn.PrintfLine("220 mailinator.local ESMTP ready"); err != nil {
		return
	}

	for {
		line, err := session.conn.ReadLine()
		if err != nil {
			return
		}

		command, rest, _ := strings.Cut(line, " ")
		command = strings.ToUpper(command)

		switch command {
		case "HELO", "EHLO":
			session.handleGreet(rest)
		case "MAIL":
			session.handleMail(rest)
		case "RCPT":
			session.handleRcpt(rest)
		case "DATA":
			session.handleData()
		case "NOOP":
			session.writeResponse(250, "Ok")
		case "RSET":
			session.handleReset()
		case "QUIT":
			session.handleQuit()
		default:
			session.writeResponse(500, "command unrecognized")
		}

		if session.err != nil {
			return
		}

		if session.quit {
			return
		}
	}
}

func (s *session) writeResponse(code int, message string) {
	if err := s.conn.PrintfLine("%d %v", code, message); err != nil {
		s.err = err
	}
}

func (s *session) writeBadSequenceResponse() {
	s.writeResponse(503, "Bad sequence of commands")
}

func (s *session) writeCompletedResponse(message string) {
	s.writeResponse(250, message)
}

func (s *session) handleGreet(argument string) {
	if s.state != stateNotYetGreeted {
		s.writeBadSequenceResponse()
		return
	}

	s.helo = strings.TrimSpace(argument)
	s.writeCompletedResponse("mailinator.local")
	s.state = stateGreeted
}

func (s *session) handleMail(argument string) {
	if s.state != stateGreeted {
		s.writeBadSequenceResponse()
		return
	}

	_, afterBracket, _ := strings.Cut(argument, "<")
	addr, _, _ := strings.Cut(afterBracket, ">")
	addr = strings.TrimSpace(addr)
	if addr == "" {
		s.writeResponse(501, "Syntax error in parameters")
		return
	}

	s.sender = addr

	s.writeCompletedResponse("Sender ok")
	s.state = stateMailFromSet
}

func (s *session) handleRcpt(argument string) {
	if s.state != stateMailFromSet && s.state != stateRcptSet {
		s.writeBadSequenceResponse()
		return
	}

	_, afterBracket, _ := strings.Cut(argument, "<")
	addr, _, _ := strings.Cut(afterBracket, ">")
	addr = strings.TrimSpace(addr)
	if addr == "" {
		s.writeResponse(501, "Syntax error in parameters")
		return
	}

	s.recipients = append(s.recipients, addr)

	s.writeCompletedResponse("Recipient ok")
	s.state = stateRcptSet
}

func (s *session) handleData() {
	if s.state != stateRcptSet {
		s.writeBadSequenceResponse()
		return
	}

	if err := s.conn.PrintfLine("354 Start mail input; end with <CRLF>.<CRLF>"); err != nil {
		s.err = err
		return
	}

	dotReader := s.conn.DotReader()
	limited := io.LimitReader(dotReader, s.maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		s.err = err
		return
	}

	if int64(len(data)) > s.maxSize {
		if _, err := io.Copy(io.Discard, dotReader); err != nil {
			s.err = err
			return
		}

		s.writeResponse(552, "Message size exceeds fixed maximum message size")
		s.sender = ""
		s.recipients = []string{}
		s.state = stateGreeted
		return
	}

	// store mail
	s.message = string(data)

	s.writeCompletedResponse("queued")
	s.sender = ""
	s.recipients = []string{}
	s.message = ""
	s.state = stateGreeted
}

func (s *session) handleReset() {
	if s.state == stateNotYetGreeted {
		s.writeBadSequenceResponse()
		return
	}

	s.sender = ""
	s.recipients = []string{}
	s.message = ""
	s.state = stateGreeted

	s.writeCompletedResponse("OK")
}

func (s *session) handleQuit() {
	s.writeResponse(221, "Bye")
	s.quit = true
}
