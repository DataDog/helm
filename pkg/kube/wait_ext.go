package kube

import appsv1 "k8s.io/api/apps/v1"

// statefulSetReadyCompat is statefulSetReady, plus local changes to support our 1.10 k8s clusters
func (w *waiter) statefulSetReadyCompat(sts *appsv1.StatefulSet) bool {
	// If the update strategy is not a rolling update, there will be nothing to wait for
	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		return true
	}

	// Dereference all the pointers because StatefulSets like them
	var partition int
	// 1 is the default for replicas if not set
	var replicas = 1
	// On Kubernetes >= 1.11, UpdatedReplicas reflects the number of updated pods
	var updated = int(sts.Status.UpdatedReplicas)
	// For some reason, even if the update strategy is a rolling update, the
	// actual rollingUpdate field can be nil. If it is, we can safely assume
	// there is no partition value
	if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		partition = int(*sts.Spec.UpdateStrategy.RollingUpdate.Partition)
	}
	if sts.Spec.Replicas != nil {
		replicas = int(*sts.Spec.Replicas)
	}

	// Because an update strategy can use partitioning, we need to calculate the
	// number of updated replicas we should have. For example, if the replicas
	// is set to 3 and the partition is 2, we'd expect only one pod to be
	// updated
	expectedReplicas := replicas - partition

	// On old (< 1.11) Kubernetes versions, UpdatedReplicas is reset to 0 after a
	// successful rollout. That's why we need to verify and re-evaluate that count
	if updated == 0 {
		pods, err := w.podsforObject(sts.GetNamespace(), sts)
		if err != nil {
			w.log("Failed to fetch pods for StatefulSet %s/%s (will retry): %s", sts.Namespace, sts.Name, err)
			return false
		}

		for _, pod := range pods {
			if hash, ok := pod.GetLabels()["controller-revision-hash"]; ok {
				if hash == sts.Status.UpdateRevision {
					updated++
				}
			}
		}
	}

	// Make sure all the updated pods have been scheduled
	if updated != expectedReplicas {
		w.log("StatefulSet is not ready: %s/%s. %d out of %d expected pods have been scheduled", sts.Namespace, sts.Name, sts.Status.UpdatedReplicas, expectedReplicas)
		return false
	}

	if int(sts.Status.ReadyReplicas) != replicas {
		w.log("StatefulSet is not ready: %s/%s. %d out of %d expected pods are ready", sts.Namespace, sts.Name, sts.Status.ReadyReplicas, replicas)
		return false
	}
	return true
}
