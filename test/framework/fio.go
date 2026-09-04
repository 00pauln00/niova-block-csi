package framework

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FIOResult holds the parsed output of a fio JSON run.
type FIOResult struct {
	Jobs []FIOJob `json:"jobs"`
}

type FIOJob struct {
	JobName string  `json:"jobname"`
	Error   int     `json:"error"`
	Read    FIOStat `json:"read"`
	Write   FIOStat `json:"write"`
}

type FIOStat struct {
	IOPS      float64    `json:"iops"`
	BW        float64    `json:"bw"`        // KiB/s
	LatNs     FIOLatency `json:"lat_ns"`
}

type FIOLatency struct {
	Mean   float64 `json:"mean"`
	Stddev float64 `json:"stddev"`
	Percentile map[string]float64 `json:"percentile"`
}

// RunFIOVerify writes data with checksums then reads back to verify integrity.
// target is either /dev/test-block (block PVC) or /data/test.img (filesystem PVC).
// Returns an error if any verify failures are detected.
func (f *Framework) RunFIOVerify(podName, target, size string) error {
	writeCmd := fioCmd(target, "randwrite", size, "--verify=sha512", "--verify_fatal=1")
	Logf("fio write+verify on %s: %s", podName, strings.Join(writeCmd, " "))
	out, err := f.ExecInPod(podName, "test", writeCmd)
	if err != nil {
		return fmt.Errorf("fio write failed: %v\noutput: %s", err, out)
	}
	if strings.Contains(out, "verify failed") || strings.Contains(out, "VERIFY FAILED") {
		return fmt.Errorf("fio verify errors detected:\n%s", out)
	}

	readCmd := fioCmd(target, "randread", size, "--verify=sha512", "--verify_fatal=1", "--verify_only")
	Logf("fio read-verify on %s: %s", podName, strings.Join(readCmd, " "))
	out, err = f.ExecInPod(podName, "test", readCmd)
	if err != nil {
		return fmt.Errorf("fio verify-read failed: %v\noutput: %s", err, out)
	}
	if strings.Contains(out, "verify failed") || strings.Contains(out, "VERIFY FAILED") {
		return fmt.Errorf("fio read verify errors:\n%s", out)
	}
	return nil
}

// RunFIOBenchmark runs a fio benchmark and returns parsed results.
func (f *Framework) RunFIOBenchmark(podName, target, rw, bs, size string, iodepth int) (*FIOResult, error) {
	cmd := fioCmd(target, rw, size,
		"--bs="+bs,
		fmt.Sprintf("--iodepth=%d", iodepth),
		"--output-format=json",
	)
	Logf("fio benchmark on %s: %s", podName, strings.Join(cmd, " "))
	out, err := f.ExecInPod(podName, "test", cmd)
	if err != nil {
		return nil, fmt.Errorf("fio benchmark failed: %v\noutput: %s", err, out)
	}

	// fio may emit non-JSON lines before the JSON block; find the opening brace.
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		return nil, fmt.Errorf("no JSON in fio output: %s", out)
	}

	var result FIOResult
	if err := json.Unmarshal([]byte(out[jsonStart:]), &result); err != nil {
		return nil, fmt.Errorf("parsing fio JSON: %v\nraw: %s", err, out)
	}
	return &result, nil
}

func fioCmd(target, rw, size string, extra ...string) []string {
	cmd := []string{
		"fio",
		"--name=test",
		"--filename=" + target,
		"--rw=" + rw,
		"--size=" + size,
		"--direct=1",
		"--ioengine=libaio",
		"--iodepth=16",
		"--bs=4k",
		"--group_reporting",
	}
	return append(cmd, extra...)
}
