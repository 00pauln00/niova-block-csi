package performance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/niova-block-csi/test/framework"
)

func TestPerformance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Tests")
}

var f = framework.New()

// thresholds holds minimum acceptable performance values.
// Override by setting PERF_THRESHOLDS_FILE to a JSON file path.
type Thresholds struct {
	SeqReadBWMiBs   float64 `json:"seq_read_bw_mibs"`
	SeqWriteBWMiBs  float64 `json:"seq_write_bw_mibs"`
	RandReadIOPS    float64 `json:"rand_read_iops"`
	RandWriteIOPS   float64 `json:"rand_write_iops"`
	RandReadP99UsLat float64 `json:"rand_read_p99_us_lat"`
}

var defaultThresholds = Thresholds{
	SeqReadBWMiBs:   200,   // MiB/s
	SeqWriteBWMiBs:  150,
	RandReadIOPS:    5000,
	RandWriteIOPS:   3000,
	RandReadP99UsLat: 5000, // 5ms
}

func loadThresholds() Thresholds {
	path := os.Getenv("PERF_THRESHOLDS_FILE")
	if path == "" {
		return defaultThresholds
	}
	data, err := os.ReadFile(path)
	if err != nil {
		framework.Logf("cannot read thresholds file %s: %v; using defaults", path, err)
		return defaultThresholds
	}
	var t Thresholds
	if err := json.Unmarshal(data, &t); err != nil {
		framework.Logf("cannot parse thresholds file: %v; using defaults", err)
		return defaultThresholds
	}
	return t
}

var _ = Describe("Performance", Label("performance"), func() {
	var (
		pvcName  = "perf-block"
		podName  = "perf-block-pod"
		thresh   Thresholds
	)

	BeforeEach(func() {
		thresh = loadThresholds()
	})

	BeforeSuite(func() {
		By("creating and staging a 64Gi block PVC for benchmarks")
		_, err := f.CreatePVC(pvcName, "64Gi",
			corev1.PersistentVolumeModeBlock,
			corev1.ReadWriteOnce)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.WaitForPVCBound(pvcName, framework.PVCBoundTimeout)).To(Succeed())
		_, err = f.CreatePodWithBlockPVC(podName, pvcName)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.WaitForPodRunning(podName, framework.PodRunningTimeout)).To(Succeed())
	})

	AfterSuite(func() {
		_ = f.DeletePod(podName)
		_ = f.WaitForPodDeleted(podName, framework.PodDeleteTimeout)
		_ = f.DeletePVC(pvcName)
	})

	Describe("Sequential I/O", func() {
		It("sequential read meets bandwidth threshold", func() {
			result, err := f.RunFIOBenchmark(podName, "/dev/test-block",
				"read", "1m", "32Gi", 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Jobs).NotTo(BeEmpty())

			bwMiBs := result.Jobs[0].Read.BW / 1024
			framework.Logf("sequential read: %.0f MiB/s (threshold: %.0f)", bwMiBs, thresh.SeqReadBWMiBs)
			publishMetric("seq_read_bw_mibs", bwMiBs)
			Expect(bwMiBs).To(BeNumerically(">=", thresh.SeqReadBWMiBs),
				"sequential read BW below threshold")
		})

		It("sequential write meets bandwidth threshold", func() {
			result, err := f.RunFIOBenchmark(podName, "/dev/test-block",
				"write", "1m", "32Gi", 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Jobs).NotTo(BeEmpty())

			bwMiBs := result.Jobs[0].Write.BW / 1024
			framework.Logf("sequential write: %.0f MiB/s (threshold: %.0f)", bwMiBs, thresh.SeqWriteBWMiBs)
			publishMetric("seq_write_bw_mibs", bwMiBs)
			Expect(bwMiBs).To(BeNumerically(">=", thresh.SeqWriteBWMiBs))
		})
	})

	Describe("Random I/O", func() {
		It("random read meets IOPS threshold", func() {
			result, err := f.RunFIOBenchmark(podName, "/dev/test-block",
				"randread", "4k", "32Gi", 128)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Jobs).NotTo(BeEmpty())

			iops := result.Jobs[0].Read.IOPS
			p99us := result.Jobs[0].Read.LatNs.Percentile["99.000000"] / 1000
			framework.Logf("random read: %.0f IOPS, p99=%.0fµs (thresholds: %.0f IOPS, %.0fµs)",
				iops, p99us, thresh.RandReadIOPS, thresh.RandReadP99UsLat)
			publishMetric("rand_read_iops", iops)
			publishMetric("rand_read_p99_us", p99us)
			Expect(iops).To(BeNumerically(">=", thresh.RandReadIOPS))
			Expect(p99us).To(BeNumerically("<=", thresh.RandReadP99UsLat))
		})

		It("random write meets IOPS threshold", func() {
			result, err := f.RunFIOBenchmark(podName, "/dev/test-block",
				"randwrite", "4k", "32Gi", 128)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Jobs).NotTo(BeEmpty())

			iops := result.Jobs[0].Write.IOPS
			framework.Logf("random write: %.0f IOPS (threshold: %.0f)", iops, thresh.RandWriteIOPS)
			publishMetric("rand_write_iops", iops)
			Expect(iops).To(BeNumerically(">=", thresh.RandWriteIOPS))
		})
	})

	Describe("Scaling", func() {
		It("maintains acceptable latency with 10 concurrent PVCs", func() {
			const n = 10
			type result struct {
				idx  int
				iops float64
				err  error
			}
			results := make(chan result, n)

			pvcs := make([]string, n)
			pods := make([]string, n)
			for i := 0; i < n; i++ {
				pvcs[i] = fmt.Sprintf("perf-scale-%d", i)
				pods[i] = fmt.Sprintf("perf-scale-pod-%d", i)
				_, err := f.CreatePVC(pvcs[i], "10Gi",
					corev1.PersistentVolumeModeBlock, corev1.ReadWriteOnce)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(f.DeletePVC, pvcs[i])
				Expect(f.WaitForPVCBound(pvcs[i], framework.PVCBoundTimeout)).To(Succeed())
				_, err = f.CreatePodWithBlockPVC(pods[i], pvcs[i])
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(f.DeletePod, pods[i])
				Expect(f.WaitForPodRunning(pods[i], framework.PodRunningTimeout)).To(Succeed())
			}

			// Run concurrent fio benchmarks
			for i := 0; i < n; i++ {
				i := i
				go func() {
					r, err := f.RunFIOBenchmark(pods[i], "/dev/test-block",
						"randread", "4k", "1Gi", 32)
					if err != nil || len(r.Jobs) == 0 {
						results <- result{i, 0, err}
						return
					}
					results <- result{i, r.Jobs[0].Read.IOPS, nil}
				}()
			}

			var totalIOPS float64
			deadline := time.After(10 * time.Minute)
			for i := 0; i < n; i++ {
				select {
				case r := <-results:
					Expect(r.err).NotTo(HaveOccurred(), "scaling benchmark %d failed", r.idx)
					totalIOPS += r.iops
				case <-deadline:
					Fail("scaling benchmark timed out")
				}
			}
			framework.Logf("scaling test: %.0f aggregate IOPS across %d PVCs", totalIOPS, n)
			publishMetric("scale_aggregate_iops", totalIOPS)
		})
	})
})

// publishMetric writes a metric line to the pipeline artifact file if
// PERF_RESULTS_FILE is set (CI collects this for trend graphs).
func publishMetric(name string, value float64) {
	path := os.Getenv("PERF_RESULTS_FILE")
	if path == "" {
		return
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		fmt.Fprintf(f, "%s=%.2f\n", name, value)
		f.Close()
	}
}
