package process

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	uuid "github.com/satori/go.uuid"
)

// OutputChunk represents a chunk of output for the process
type OutputChunk struct {
	Chunk string
	Full  string
}

// Process represents a more observable exec.Cmd command
type Process struct {
	Cmd       *exec.Cmd
	Output    string
	StdinPipe io.WriteCloser

	outputChannels map[string]chan OutputChunk
	mutex          sync.Mutex // lock for updating Output and outputChannels
}

// NewProcess is Process's constructor
func NewProcess(commandWords ...string) *Process {
	p := &Process{
		Cmd:            exec.Command(commandWords[0], commandWords[1:]...), //nolint gas
		mutex:          sync.Mutex{},
		outputChannels: map[string]chan OutputChunk{},
	}
	return p
}

// Kill kills the process if it is running
func (p *Process) Kill() error {
	if p.isRunning() {
		return p.Cmd.Process.Kill()
	}
	return nil
}

// GetOutputChannel returns a channel that passes OutputChunk as they occur
// it will immediately send an OutputChunk with an empty chunk and the full output
// thus far. It also returns a function that when called closes the channel
func (p *Process) GetOutputChannel() (chan OutputChunk, func()) {
	id := uuid.NewV4().String()
	p.mutex.Lock()
	p.outputChannels[id] = make(chan OutputChunk)
	p.sendOutputChunk(p.outputChannels[id], OutputChunk{Full: p.Output})
	p.mutex.Unlock()
	return p.outputChannels[id], func() {
		p.mutex.Lock()
		delete(p.outputChannels, id)
		p.mutex.Unlock()
	}
}

// Run is shorthand for Start() followed by Wait()
func (p *Process) Run() error {
	if err := p.Start(); err != nil {
		return err
	}
	return p.Wait()
}

// SetDir sets the directory that the process should be run in
func (p *Process) SetDir(dir string) {
	p.Cmd.Dir = dir
}

// SetEnv sets the environment for the process
func (p *Process) SetEnv(env []string) {
	p.Cmd.Env = env
}

// Start runs the process and returns an error if any
func (p *Process) Start() error {
	var err error
	p.StdinPipe, err = p.Cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdoutPipe, err := p.Cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go p.log(stdoutPipe)
	stderrPipe, err := p.Cmd.StderrPipe()
	if err != nil {
		return err
	}
	go p.log(stderrPipe)
	return p.Cmd.Start()
}

// Wait waits for the process to finish, can only be called after Start()
func (p *Process) Wait() error {
	return p.Cmd.Wait()
}

// WaitForCondition calls the given function with the latest chunk of output
// and the full output until it returns true
// // returns an error if it does not match after the given duration
func (p *Process) WaitForCondition(condition func(string, string) bool, duration time.Duration) error {
	success := make(chan bool)
	go p.waitForCondition(condition, success)
	select {
	case <-success:
		return nil
	case <-time.After(duration):
		return fmt.Errorf("Timed out after %v, full output:\n%s", duration, p.Output)
	}
}

// WaitForRegexp waits for the full output to match the given regex
// returns an error if it does not match after the given duration
func (p *Process) WaitForRegexp(isValid *regexp.Regexp, duration time.Duration) error {
	return p.WaitForCondition(func(outputChunk, fullOutput string) bool {
		return isValid.MatchString(fullOutput)
	}, duration)
}

// WaitForText waits for the full output to contain the given text
// returns an error if it does not match after the given duration
func (p *Process) WaitForText(text string, duration time.Duration) error {
	return p.WaitForCondition(func(outputChunk, fullOutput string) bool {
		return strings.Contains(fullOutput, text)
	}, duration)
}

// Helpers

func (p *Process) isRunning() bool {
	err := p.Cmd.Process.Signal(syscall.Signal(0))
	return fmt.Sprint(err) != "os: process already finished"
}

func (p *Process) log(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Split(scanLinesOrPrompt)
	for scanner.Scan() {
		text := scanner.Text()
		// Its important to lock the outputMutex before the onOutputFuncsMutex
		// becuase the waitFor method locks outputMutex and then may or may not lock
		// the onOutputFuncsMutex. We need to use the same order to avoid deadlock
		p.mutex.Lock()
		if p.Output != "" {
			p.Output += "\n"
		}
		p.Output += text
		outputChunk := OutputChunk{Chunk: text, Full: p.Output}
		for _, outputChannel := range p.outputChannels {
			p.sendOutputChunk(outputChannel, outputChunk)
		}
		p.mutex.Unlock()
	}
}

func (p *Process) sendOutputChunk(outputChannel chan OutputChunk, outputChunk OutputChunk) {
	go func() {
		outputChannel <- outputChunk
	}()
}

func (p *Process) waitForCondition(condition func(string, string) bool, success chan<- bool) {
	outputChannel, stopFunc := p.GetOutputChannel()
	for {
		outputChunk := <-outputChannel
		if condition(outputChunk.Chunk, outputChunk.Full) {
			success <- true
			stopFunc()
			return
		}
	}
}
