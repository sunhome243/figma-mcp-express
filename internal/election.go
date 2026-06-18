package internal

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var electionLogger = log.New(os.Stderr, "[election] ", 0)

// Election determines the initial role and monitors leader health.
// If the leader dies, a follower will attempt a takeover.
type Election struct {
	port     int
	node     *Node
	follower *Follower // reused across health-check ticks to avoid HTTP client pool leaks
	cancel   context.CancelFunc
}

// NewElection creates an Election for the given ip, port, and node.
func NewElection(ip string, port int, node *Node) *Election {
	return &Election{
		port:     port,
		node:     node,
		follower: NewFollower("http://" + ip + ":" + itoa(port)),
	}
}

// Start determines the initial role and launches the background monitor.
func (e *Election) Start(ctx context.Context) error {
	if err := e.determineRole(ctx); err != nil {
		return err
	}

	monitorCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	go e.monitor(monitorCtx)
	return nil
}

// Stop cancels the background monitor goroutine.
func (e *Election) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

// determineRole tries to become leader; falls back to follower if a
// healthy leader already exists on the port — UNLESS that leader is running an
// older binary than ours, in which case we evict it (issue #26).
func (e *Election) determineRole(ctx context.Context) error {
	if err := e.node.BecomeLeader(); err == nil {
		return nil
	}

	// Port taken — check if there is a healthy leader and what version it runs.
	healthy, remoteVersion := e.follower.PingVersion(ctx)
	if healthy {
		// Stale-binary guard: a strictly-newer local binary must not proxy to an
		// older leader forever. Evict it and take over. Equal/older/unparseable
		// ("dev") → follow (no fight, no flapping).
		if shouldTakeOver(e.node.version, remoteVersion) {
			electionLogger.Printf("local version %s newer than leader %s — evicting stale leader and taking over",
				e.node.version, remoteVersion)
			return e.takeOver(ctx, "stale leader")
		}
		e.node.BecomeFollower()
		return nil
	}

	// Port taken but no healthy leader — zombie process holding the port.
	electionLogger.Printf("port taken but leader not responding — killing zombie and taking over")
	return e.takeOver(ctx, "zombie")
}

// takeOver kills whatever process holds the port and re-attempts BecomeLeader.
// Best-effort: on failure it logs and leaves the role unchanged for the next
// monitor tick to retry. Shared by the zombie path and the stale-version path
// (issue #26). reason is logged for diagnosis.
func (e *Election) takeOver(ctx context.Context, reason string) error {
	killPortHolder(e.port)
	select {
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := e.node.BecomeLeader(); err == nil {
		return nil
	}
	electionLogger.Printf("takeover failed after killing %s — will retry on next tick", reason)
	return nil
}

// monitor runs a periodic check on the current role.
// Followers watch the leader; leaders do nothing.
func (e *Election) monitor(ctx context.Context) {
	for {
		// Jitter: 3–5 seconds
		jitter := time.Duration(3000+rand.Intn(2000)) * time.Millisecond
		select {
		case <-time.After(jitter):
		case <-ctx.Done():
			return
		}

		if err := e.tick(ctx); err != nil {
			electionLogger.Printf("tick error: %v", err)
		}
	}
}

func (e *Election) tick(ctx context.Context) error {
	switch e.node.Role() {
	case RoleFollower:
		healthy, remoteVersion := e.follower.PingVersion(ctx)
		if !healthy {
			electionLogger.Printf("leader not responding, attempting takeover...")
			if err := e.node.BecomeLeader(); err != nil {
				electionLogger.Printf("takeover failed: %v", err)
			}
		} else if shouldTakeOver(e.node.version, remoteVersion) {
			// We discovered (after following) that the leader runs an older binary
			// than ours — evict it (issue #26).
			electionLogger.Printf("leader %s older than local %s — evicting stale leader",
				remoteVersion, e.node.version)
			if err := e.takeOver(ctx, "stale leader"); err != nil {
				electionLogger.Printf("stale-leader takeover error: %v", err)
			}
		}
	case RoleUnknown:
		return e.determineRole(ctx)
	case RoleLeader:
		// Nothing — we are the leader
	}
	return nil
}

// itoa converts an int to string without importing strconv everywhere.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// killPortHolder finds and terminates the process listening on the given port.
// Uses lsof to find the PID, sends SIGTERM (falls back to SIGKILL), and skips
// the current process. Best-effort: logs but does not return errors.
func killPortHolder(port int) {
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port), "-sTCP:LISTEN").Output()
	if err != nil {
		electionLogger.Printf("killPortHolder: lsof failed: %v", err)
		return
	}
	self := os.Getpid()
	for _, pidStr := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == self {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			electionLogger.Printf("killPortHolder: FindProcess %d: %v", pid, err)
			continue
		}
		electionLogger.Printf("killPortHolder: sending SIGTERM to zombie PID %d on :%d", pid, port)
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			electionLogger.Printf("killPortHolder: SIGTERM failed, trying SIGKILL: %v", err)
			_ = proc.Kill()
		}
	}
}
