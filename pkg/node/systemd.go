package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
	"k8s.io/klog/v2"
)

const ublkSystemdSlice = "niova-ublk.slice"

// ublkUnitName derives a deterministic systemd unit name from the volume
// ID, so start/stop never need to remember state across a node-plugin
// restart — NodeUnstageVolume always hands the volumeID back, which is
// enough to recompute the unit to stop.
func ublkUnitName(volumeID string) string {
	return fmt.Sprintf("niova-ublk-%s.service", volumeID)
}

// startUblkUnit asks the host's systemd to fork/exec niova-ublk as a
// transient service. Because systemd — not this container — does the
// forking, the process is never a child of the node-plugin's process
// tree: restarting the node-plugin container (crash, OOM, image upgrade)
// does not affect it. Returns the daemon's PID.
func startUblkUnit(volumeID, binary string, args []string, env []string, dir string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := dbus.NewSystemdConnectionContext(ctx)
	if err != nil {
		return -1, fmt.Errorf("failed to connect to systemd: %v", err)
	}
	defer conn.Close()

	unitName := ublkUnitName(volumeID)
	execArgs := append([]string{binary}, args...)

	props := []dbus.Property{
		dbus.PropDescription(fmt.Sprintf("niova-ublk daemon for volume %s", volumeID)),
		dbus.PropType("simple"),
		dbus.PropExecStart(execArgs, true),
		dbus.PropSlice(ublkSystemdSlice),
		{Name: "Environment", Value: godbus.MakeVariant(env)},
		{Name: "WorkingDirectory", Value: godbus.MakeVariant(dir)},
	}

	resultChan := make(chan string, 1)
	if _, err := conn.StartTransientUnitContext(ctx, unitName, "replace", props, resultChan); err != nil {
		return -1, fmt.Errorf("failed to start transient unit %s: %v", unitName, err)
	}

	select {
	case result := <-resultChan:
		if result != "done" {
			return -1, fmt.Errorf("systemd job for unit %s finished with result %q", unitName, result)
		}
	case <-ctx.Done():
		return -1, fmt.Errorf("timed out waiting for systemd to start unit %s", unitName)
	}

	prop, err := conn.GetServicePropertyContext(ctx, unitName, "MainPID")
	if err != nil {
		return -1, fmt.Errorf("failed to get MainPID for unit %s: %v", unitName, err)
	}
	pid, ok := prop.Value.Value().(uint32)
	if !ok || pid == 0 {
		return -1, fmt.Errorf("unit %s has no live MainPID after start", unitName)
	}

	klog.Infof("Started niova-ublk as systemd unit %s (pid %d) for volume %s", unitName, pid, volumeID)
	return int(pid), nil
}

// stopUblkUnit stops the systemd unit for volumeID, if it exists. systemd
// sends SIGTERM, escalates to SIGKILL on timeout, and cleans up the
// transient unit and its cgroup — no PID bookkeeping required on our
// side, and this works even if the node plugin restarted in between,
// since the unit name is recomputed from volumeID rather than remembered.
func stopUblkUnit(volumeID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := dbus.NewSystemdConnectionContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to systemd: %v", err)
	}
	defer conn.Close()

	unitName := ublkUnitName(volumeID)
	resultChan := make(chan string, 1)
	if _, err := conn.StopUnitContext(ctx, unitName, "replace", resultChan); err != nil {
		var dbusErr godbus.Error
		if errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit" {
			klog.Infof("systemd unit %s not found, considering it already stopped", unitName)
			return nil
		}
		return fmt.Errorf("failed to stop unit %s: %v", unitName, err)
	}

	select {
	case result := <-resultChan:
		if result != "done" {
			return fmt.Errorf("systemd job to stop unit %s finished with result %q", unitName, result)
		}
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for systemd to stop unit %s", unitName)
	}

	klog.Infof("Stopped systemd unit %s", unitName)
	return nil
}
