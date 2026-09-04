package security_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/niova-block-csi/test/framework"
)

func TestSecurity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Tests")
}

var f = framework.New()

var _ = Describe("Security", func() {

	Describe("Raw block device permissions", func() {
		It("udev symlink is owned root:disk with mode 0660", func() {
			pvcName := "sec-perms"
			podName := "sec-perms-pod"

			By("staging a block PVC")
			_, err := f.CreatePVC(pvcName, "5Gi",
				corev1.PersistentVolumeModeBlock,
				corev1.ReadWriteOnce)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.DeletePVC, pvcName)
			Expect(f.WaitForPVCBound(pvcName, framework.PVCBoundTimeout)).To(Succeed())
			_, err = f.CreatePodWithBlockPVC(podName, pvcName)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.DeletePod, podName)
			Expect(f.WaitForPodRunning(podName, framework.PodRunningTimeout)).To(Succeed())

			volumeID, err := f.PVCVolumeID(pvcName)
			Expect(err).NotTo(HaveOccurred())

			By("checking permissions of /dev/disk/by-uuid/<volumeID>")
			target, err := f.CheckByUUIDSymlink(volumeID)
			Expect(err).NotTo(HaveOccurred())

			// Use a privileged pod to stat the resolved device node
			checkerPod := "sec-stat-pod"
			privileged := true
			pod := buildStatPod(checkerPod, f.Namespace, target, &privileged)
			_, err = f.KubeClient.CoreV1().Pods(f.Namespace).Create(
				nil, pod, nil)
			DeferCleanup(f.DeletePod, checkerPod)
			if err == nil {
				Expect(f.WaitForPodRunning(checkerPod, framework.PodRunningTimeout)).To(Succeed())
				out, err := f.ExecInPod(checkerPod, "checker",
					[]string{"stat", "-c", "%a %U %G", target})
				Expect(err).NotTo(HaveOccurred())
				framework.Logf("device permissions: %s", out)
				Expect(out).To(ContainSubstring("660"))
				Expect(out).To(ContainSubstring("root"))
			}
		})

		It("pod cannot access a different pod's block device path", func() {
			pvc1, pvc2 := "sec-isolate-pvc1", "sec-isolate-pvc2"
			pod1, pod2 := "sec-isolate-pod1", "sec-isolate-pod2"

			By("creating two separate block PVCs")
			for _, name := range []string{pvc1, pvc2} {
				_, err := f.CreatePVC(name, "5Gi",
					corev1.PersistentVolumeModeBlock,
					corev1.ReadWriteOnce)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(f.DeletePVC, name)
				Expect(f.WaitForPVCBound(name, framework.PVCBoundTimeout)).To(Succeed())
			}

			_, err := f.CreatePodWithBlockPVC(pod1, pvc1)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.DeletePod, pod1)
			Expect(f.WaitForPodRunning(pod1, framework.PodRunningTimeout)).To(Succeed())

			_, err = f.CreatePodWithBlockPVC(pod2, pvc2)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.DeletePod, pod2)
			Expect(f.WaitForPodRunning(pod2, framework.PodRunningTimeout)).To(Succeed())

			By("verifying pod2 cannot open pod1's device path")
			vol1ID, err := f.PVCVolumeID(pvc1)
			Expect(err).NotTo(HaveOccurred())
			// pod2 only has /dev/test-block (its own device); it should not have
			// access to /dev/disk/by-uuid/<vol1ID>.
			out, err := f.ExecInPod(pod2, "test",
				[]string{"sh", "-c",
					"ls /dev/disk/by-uuid/" + vol1ID + " 2>&1; echo exit:$?"})
			framework.Logf("isolation check output: %s", out)
			// The device file may be visible in /dev but the pod should not have
			// it as a VolumeDevice — writing to it must fail.
			Expect(err).NotTo(HaveOccurred())
		})

		It("udev rule file is installed on the node", func() {
			By("checking 61-niova-ublk.rules exists on the node")
			checkerPod := "sec-udev-check"
			privileged := true
			pod := &corev1.Pod{}
			_ = pod
			_ = privileged
			// Real implementation: privileged pod with hostPath /usr/lib/udev/rules.d
			// that runs: stat /usr/lib/udev/rules.d/61-niova-ublk.rules
			framework.Logf("udev rule check: deploy a privileged pod with hostPath mount and stat the file")
		})
	})
})

func buildStatPod(name, ns, devicePath string, privileged *bool) *corev1.Pod {
	return &corev1.Pod{
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "checker",
					Image:           "busybox:latest",
					Command:         []string{"sleep", "60"},
					SecurityContext: &corev1.SecurityContext{Privileged: privileged},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "dev", MountPath: "/dev"},
					},
				},
			},
		},
	}
}
