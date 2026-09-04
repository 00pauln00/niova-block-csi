package framework

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// CheckByUUIDSymlink verifies /dev/disk/by-uuid/<volumeID> exists on the node
// by running a privileged pod. Returns the symlink target (e.g. ../../ublkb0).
func (f *Framework) CheckByUUIDSymlink(volumeID string) (string, error) {
	podName := "check-uuid-" + volumeID[:8]
	privileged := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: f.Namespace},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			NodeName:      f.NodeName,
			Containers: []corev1.Container{
				{
					Name:    "checker",
					Image:   "busybox:latest",
					Command: []string{"sleep", "60"},
					SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "dev", MountPath: "/dev"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "dev",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{Path: "/dev"},
					},
				},
			},
		},
	}

	_, err := f.KubeClient.CoreV1().Pods(f.Namespace).Create(
		context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	defer f.DeletePod(podName)

	if err := f.WaitForPodRunning(podName, 60*time.Second); err != nil {
		return "", fmt.Errorf("checker pod not running: %v", err)
	}

	out, err := f.ExecInPod(podName, "checker",
		[]string{"readlink", "-f", "/dev/disk/by-uuid/" + volumeID})
	if err != nil {
		return "", fmt.Errorf("by-uuid symlink not found for %s: %v", volumeID, err)
	}
	return strings.TrimSpace(out), nil
}

// RestartCSIDaemonSetPod deletes the CSI node pod on f.NodeName, triggering a
// restart. It waits until the replacement pod is Running before returning.
func (f *Framework) RestartCSIDaemonSetPod() error {
	ctx := context.Background()
	pods, err := f.KubeClient.CoreV1().Pods(DefaultCSINamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + CSIDaemonSetName,
	})
	if err != nil || len(pods.Items) == 0 {
		return fmt.Errorf("no CSI pods found: %v", err)
	}

	var target *corev1.Pod
	for i := range pods.Items {
		if f.NodeName == "" || pods.Items[i].Spec.NodeName == f.NodeName {
			target = &pods.Items[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no CSI pod found on node %s", f.NodeName)
	}

	grace := int64(0)
	Logf("deleting CSI pod %s to trigger restart", target.Name)
	if err := f.KubeClient.CoreV1().Pods(DefaultCSINamespace).Delete(
		ctx, target.Name, metav1.DeleteOptions{GracePeriodSeconds: &grace}); err != nil {
		return err
	}

	// Wait for the replacement pod to become Running.
	return wait.PollUntilContextTimeout(ctx, PollInterval, 2*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			pods, err := f.KubeClient.CoreV1().Pods(DefaultCSINamespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=" + CSIDaemonSetName,
			})
			if err != nil {
				return false, nil
			}
			for _, p := range pods.Items {
				if (f.NodeName == "" || p.Spec.NodeName == f.NodeName) &&
					p.Name != target.Name &&
					p.Status.Phase == corev1.PodRunning {
					Logf("replacement CSI pod %s is Running", p.Name)
					return true, nil
				}
			}
			return false, nil
		},
	)
}

// KillUblkProcess sends SIGKILL to the niova-ublk process for a given volumeID
// on the node, simulating a daemon crash. It does this via a privileged pod
// that can see the host PID namespace.
func (f *Framework) KillUblkProcess(volumeID string) error {
	podName := "kill-ublk-" + volumeID[:8]
	privileged := true
	hostPID := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: f.Namespace},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			NodeName:      f.NodeName,
			HostPID:       hostPID,
			Containers: []corev1.Container{
				{
					Name:            "killer",
					Image:           "busybox:latest",
					Command:         []string{"sleep", "60"},
					SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
				},
			},
		},
	}
	_, err := f.KubeClient.CoreV1().Pods(f.Namespace).Create(
		context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	defer f.DeletePod(podName)

	if err := f.WaitForPodRunning(podName, 60*time.Second); err != nil {
		return err
	}

	// Find and kill the niova-ublk process serving this volume.
	out, err := f.ExecInPod(podName, "killer",
		[]string{"sh", "-c", "pgrep -f 'niova-ublk.*" + volumeID + "'"})
	if err != nil || strings.TrimSpace(out) == "" {
		return fmt.Errorf("niova-ublk process for volume %s not found: %v", volumeID, err)
	}
	pid := strings.TrimSpace(out)
	Logf("killing niova-ublk pid %s for volume %s", pid, volumeID)
	_, err = f.ExecInPod(podName, "killer", []string{"kill", "-9", pid})
	return err
}
