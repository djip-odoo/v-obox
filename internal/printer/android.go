//go:build android

package printer

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type ConnKind int

const (
	ConnKindUSB ConnKind = iota
	ConnKindLAN
)

const (
	QueueSize    = 100
	WriteTimeout = 5 * time.Second
	ChunkSize    = 8 * 1024 // 8 KB
)

var (
	ErrNotFound  = errors.New("printer not found")
	ErrQueueFull = errors.New("printer queue is full")
)

type JobResult struct {
	OK  bool
	Err error
}

type JobFunc func(p *Printer) JobResult

type Job struct {
	run   JobFunc
	reply chan JobResult
}

type Info struct {
	Id   string
	Name string
	Type Type
}

type UnavailableInfo struct {
	Name  string
	Error string
}

type Printers struct {
	Available   []Info
	Unavailable []UnavailableInfo
}

func ListUSBPrinters() (*Printers, error) {
	return &Printers{
		Available:   make([]Info, 0),
		Unavailable: make([]UnavailableInfo, 0),
	}, nil
}

type Printer struct {
	connectionType ConnKind
	lanIP          string
	mu             sync.Mutex
	tcpConn        net.Conn
	jobs           chan Job
}

func newPrinter(id string) *Printer {
	if lanIP, ok := DecodeLANPrinterID(id); ok {
		p := &Printer{
			connectionType: ConnKindLAN,
			lanIP:          lanIP,
			jobs:           make(chan Job, QueueSize),
		}
		go p.loop()
		return p
	}

	p := &Printer{
		connectionType: ConnKindUSB,
		jobs:           make(chan Job, QueueSize),
	}
	go p.loop()
	return p
}

func (p *Printer) ensureOpen() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.connectionType == ConnKindLAN {
		if p.tcpConn != nil {
			return nil
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.Dial("tcp", net.JoinHostPort(p.lanIP, "9100"))
		if err != nil {
			return fmt.Errorf("failed to connect to LAN printer at %s:9100: %w", p.lanIP, err)
		}
		p.tcpConn = conn
		return nil
	}

	return ErrNotFound
}

func (p *Printer) Write(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.connectionType == ConnKindLAN {
		if p.tcpConn == nil {
			return errors.New("LAN connection not open")
		}
		if err := p.tcpConn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
			return fmt.Errorf("failed to set write deadline: %w", err)
		}
		totalWritten := 0
		for totalWritten < len(data) {
			end := totalWritten + ChunkSize
			if end > len(data) {
				end = len(data)
			}
			n, err := p.tcpConn.Write(data[totalWritten:end])
			totalWritten += n
			if err != nil {
				_ = p.tcpConn.Close()
				p.tcpConn = nil
				return fmt.Errorf("LAN write failed: %w", err)
			}
		}
		return nil
	}

	return ErrNotFound
}

func (p *Printer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.tcpConn != nil {
		_ = p.tcpConn.Close()
		p.tcpConn = nil
	}
}

func (p *Printer) Enqueue(run JobFunc, reply chan JobResult) error {
	select {
	case p.jobs <- Job{run: run, reply: reply}:
		return nil
	default:
		return ErrQueueFull
	}
}

func (p *Printer) loop() {
	for job := range p.jobs {
		result := job.run(p)
		if job.reply != nil {
			job.reply <- result
		}
	}
}
