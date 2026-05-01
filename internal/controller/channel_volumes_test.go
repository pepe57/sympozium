package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	sympoziumv1alpha1 "github.com/sympozium-ai/sympozium/api/v1alpha1"
)

// TestBuildChannelDeployment_PropagatesVolumes verifies that ChannelSpec
// Volumes/VolumeMounts are appended to the channel pod template and the
// channel container respectively.
func TestBuildChannelDeployment_PropagatesVolumes(t *testing.T) {
	r := &AgentReconciler{}
	ch := sympoziumv1alpha1.ChannelSpec{
		Type: "slack",
		Volumes: []corev1.Volume{
			{
				Name: "vault",
				VolumeSource: corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver: "secrets-store.csi.k8s.io",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "vault", MountPath: "/mnt/vault", ReadOnly: true},
		},
	}
	instance := newTestInstance()
	deploy := r.buildChannelDeployment(instance, ch, "test-instance-channel-slack")

	if got := len(deploy.Spec.Template.Spec.Volumes); got != 1 {
		t.Fatalf("pod volumes length = %d, want 1", got)
	}
	if name := deploy.Spec.Template.Spec.Volumes[0].Name; name != "vault" {
		t.Errorf("pod volume name = %q, want %q", name, "vault")
	}

	mounts := deploy.Spec.Template.Spec.Containers[0].VolumeMounts
	if len(mounts) != 1 {
		t.Fatalf("container volume mounts length = %d, want 1", len(mounts))
	}
	if mounts[0].Name != "vault" || mounts[0].MountPath != "/mnt/vault" {
		t.Errorf("mount = %+v, want name=vault mountPath=/mnt/vault", mounts[0])
	}
}

// TestBuildChannelDeployment_WhatsAppVolumesAppend verifies that per-channel
// volumes are appended alongside the WhatsApp built-in PVC volume rather than
// overwriting it.
func TestBuildChannelDeployment_WhatsAppVolumesAppend(t *testing.T) {
	r := &AgentReconciler{}
	ch := sympoziumv1alpha1.ChannelSpec{
		Type: "whatsapp",
		Volumes: []corev1.Volume{
			{Name: "vault", VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{Driver: "secrets-store.csi.k8s.io"},
			}},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "vault", MountPath: "/mnt/vault"},
		},
	}
	instance := newTestInstance()
	deploy := r.buildChannelDeployment(instance, ch, "test-instance-channel-whatsapp")

	if got := len(deploy.Spec.Template.Spec.Volumes); got != 2 {
		t.Fatalf("pod volumes length = %d, want 2 (whatsapp-data + vault)", got)
	}
	if got := len(deploy.Spec.Template.Spec.Containers[0].VolumeMounts); got != 2 {
		t.Fatalf("container volume mounts length = %d, want 2", got)
	}
}
