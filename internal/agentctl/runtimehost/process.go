package runtimehost

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// childProcess owns stdio shutdown and process termination. The inherited lease
// keeps a child fenced even if its agentctl parent is killed unexpectedly.
type childProcess struct {
	cmd     *exec.Cmd
	input   io.Closer
	outputs []io.Closer
	done    chan struct{}
	waitErr error
	once    sync.Once
}

func startChildProcess(cmd *exec.Cmd, input io.Closer, ownership *os.File, outputs ...io.Closer) (*childProcess, error) {
	if ownership != nil {
		cmd.ExtraFiles = append(cmd.ExtraFiles, ownership)
	}
	if err := cmd.Start(); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "start runtime process failed", Cause: err}
	}
	child := &childProcess{cmd: cmd, input: input, outputs: outputs, done: make(chan struct{})}
	go func() { child.waitErr = cmd.Wait(); close(child.done) }()
	return child, nil
}
func (p *childProcess) Done() <-chan struct{} { return p.done }
func (p *childProcess) Err() error {
	select {
	case <-p.done:
		if p.waitErr != nil {
			return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeInterrupted, Message: "runtime process exited unsuccessfully", Cause: p.waitErr}
		}
	default:
	}
	return nil
}

func (p *childProcess) Close() error {
	p.once.Do(func() {
		_ = p.input.Close()
		for _, output := range p.outputs {
			_ = output.Close()
		}
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-p.done:
		case <-timer.C:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	return nil
}
