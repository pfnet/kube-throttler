// Licensed to Shingo Omura under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Shingo Omura licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/everpeace/kube-throttler/pkg/apis/schedule/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
)

var _ = Describe("Clusterthrottle Test", func() {
	ctx := context.Background()
	throttleKey := "cluster-throttle"
	throttleName := "test-clusterthrottle"

	AfterEach(func() {
		MustDeleteAllClusterThrottlesInNs(ctx)
		MustDeleteAllPodsInNs(ctx, DefaultNs)
	})

	When("Pod resource usage is within its threshold", func() {
		var thr *v1alpha1.ClusterThrottle
		var pod *corev1.Pod
		BeforeEach(func() {
			thr = MustCreateClusterThrottle(ctx,
				MakeClusterThrottle(throttleName).Selector(DefaultNs, throttleKey, throttleName).
					ThresholdPod(2).
					ThresholdCpu("1").
					Obj(),
			)
			pod = MustCreatePod(ctx, MakePod(DefaultNs, "pod", "500m").Label(throttleKey, throttleName).Obj())
		})
		It("should schedule successfully", func() {
			Eventually(AsyncAll(
				WakeupBackoffPod(ctx),
				PodIsScheduled(ctx, DefaultNs, pod.Name),
				ClusterThottleHasStatus(
					ctx, thr.Name,
					ClthrOpts.WithCalculatedThreshold(thr.Spec.Threshold),
					ClthrOpts.WithUsedPod(1), ClthrOpts.WithUsedCpuReq("500m"),
					ClthrOpts.WithPodThrottled(false), ClthrOpts.WithCpuThrottled(false),
				),
			)).Should(Succeed())
		})
	})

	When("Pod resource usages exceeds threshold", func() {
		var thr *v1alpha1.ClusterThrottle
		BeforeEach(func() {
			thr = MustCreateClusterThrottle(ctx,
				MakeClusterThrottle(throttleName).Selector(DefaultNs, throttleKey, throttleName).
					ThresholdPod(2).
					ThresholdCpu("1").
					Obj(),
			)
		})
		Context("ResourceCount", func() {
			var pod1 *corev1.Pod
			var pod2 *corev1.Pod
			var pod3 *corev1.Pod
			BeforeEach(func() {
				pod1 = MustCreatePod(ctx, MakePod(DefaultNs, "pod1", "100m").Label(throttleKey, throttleName).Obj())
				pod2 = MustCreatePod(ctx, MakePod(DefaultNs, "pod2", "100m").Label(throttleKey, throttleName).Obj())
				pod3 = MustCreatePod(ctx, MakePod(DefaultNs, "pod3", "100m").Label(throttleKey, throttleName).Obj())
			})
			It("should not schedule pod3", func() {
				Eventually(AsyncAll(
					WakeupBackoffPod(ctx),
					PodIsScheduled(ctx, DefaultNs, pod1.Name),
					PodIsScheduled(ctx, DefaultNs, pod2.Name),
					ClusterThottleHasStatus(
						ctx, thr.Name,
						ClthrOpts.WithCalculatedThreshold(thr.Spec.Threshold),
						ClthrOpts.WithUsedPod(2), ClthrOpts.WithUsedCpuReq("200m"),
						ClthrOpts.WithPodThrottled(true), ClthrOpts.WithCpuThrottled(false),
					),
					MustPodFailedScheduling(ctx, DefaultNs, pod3.Name, v1alpha1.CheckThrottleStatusActive),
				)).Should(Succeed())
				Consistently(PodIsNotScheduled(ctx, DefaultNs, pod3.Name)).Should(Succeed())
			})
		})
		Context("ResourceRequest (active)", func() {
			var pod1 *corev1.Pod
			var pod2 *corev1.Pod
			BeforeEach(func() {
				pod1 = MustCreatePod(ctx, MakePod(DefaultNs, "pod1", "1").Label(throttleKey, throttleName).Obj())
				pod2 = MustCreatePod(ctx, MakePod(DefaultNs, "pod2", "500m").Label(throttleKey, throttleName).Obj())
			})
			It("should not schedule pod3", func() {
				Eventually(AsyncAll(
					WakeupBackoffPod(ctx),
					PodIsScheduled(ctx, DefaultNs, pod1.Name),
					ClusterThottleHasStatus(
						ctx, thr.Name,
						ClthrOpts.WithCalculatedThreshold(thr.Spec.Threshold),
						ClthrOpts.WithUsedPod(1), ClthrOpts.WithUsedCpuReq("1"),
						ClthrOpts.WithPodThrottled(false), ClthrOpts.WithCpuThrottled(true),
					),
					MustPodFailedScheduling(ctx, DefaultNs, pod2.Name, v1alpha1.CheckThrottleStatusActive),
				)).Should(Succeed())
				Consistently(PodIsNotScheduled(ctx, DefaultNs, pod2.Name)).Should(Succeed())
			})
		})
		Context("ResourceRequest (insufficient)", func() {
			var pod1 *corev1.Pod
			var pod2 *corev1.Pod
			BeforeEach(func() {
				pod1 = MustCreatePod(ctx, MakePod(DefaultNs, "pod1", "900m").Label(throttleKey, throttleName).Obj())
				pod2 = MustCreatePod(ctx, MakePod(DefaultNs, "pod2", "500m").Label(throttleKey, throttleName).Obj())
			})
			It("should not schedule pod3", func() {
				Eventually(AsyncAll(
					WakeupBackoffPod(ctx),
					PodIsScheduled(ctx, DefaultNs, pod1.Name),
					ClusterThottleHasStatus(
						ctx, thr.Name,
						ClthrOpts.WithCalculatedThreshold(thr.Spec.Threshold),
						ClthrOpts.WithUsedPod(1), ClthrOpts.WithUsedCpuReq("900m"),
						ClthrOpts.WithPodThrottled(false), ClthrOpts.WithCpuThrottled(false),
					),
					MustPodFailedScheduling(ctx, DefaultNs, pod2.Name, v1alpha1.CheckThrottleStatusInsufficient),
				)).Should(Succeed())
				Consistently(PodIsNotScheduled(ctx, DefaultNs, pod2.Name)).Should(Succeed())
			})
		})
		Context("ResourceRequest (pod-requests-exceeds-threshold)", func() {
			var pod *corev1.Pod
			BeforeEach(func() {
				pod = MustCreatePod(ctx, MakePod(DefaultNs, "pod", "1.1").Label(throttleKey, throttleName).Obj())
			})
			It("should not schedule pod", func() {
				Eventually(AsyncAll(
					WakeupBackoffPod(ctx),
					ClusterThottleHasStatus(
						ctx, thr.Name,
						ClthrOpts.WithCalculatedThreshold(thr.Spec.Threshold),
						ClthrOpts.WithPodThrottled(false), ClthrOpts.WithCpuThrottled(false),
					),
					MustPodFailedScheduling(ctx, DefaultNs, pod.Name, v1alpha1.CheckThrottleStatusPodRequestsExceedsThreshold),
					MustPodResourceRequestsExceedsThrottleThreshold(ctx, DefaultNs, pod.Name, thr.Namespace+"/"+thr.Name),
				)).Should(Succeed())
				Consistently(PodIsNotScheduled(ctx, DefaultNs, pod.Name)).Should(Succeed())
			})
		})
	})

	When("Group Pods", func() {
		var (
			podPassedGroup    []*corev1.Pod
			thr               *v1alpha1.ClusterThrottle
			podThrottledGroup []*corev1.Pod
		)
		BeforeEach(func() {
			thr = MustCreateClusterThrottle(ctx,
				MakeClusterThrottle(throttleName).Selector(DefaultNs, throttleKey, throttleName).
					ThresholdPod(4).
					ThresholdCpu("4").
					Obj(),
			)

			for i := 0; i < 2; i++ {
				podPassedGroup = append(podPassedGroup, MustCreatePod(ctx, MakePod(DefaultNs, fmt.Sprintf("passed-pod%d", i), "100m").Annotation(groupNameAnnotation, "passed").Label(throttleKey, throttleName).Obj()))
			}

			err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, time.Second, false, func(context.Context) (bool, error) {
				for _, pod := range podPassedGroup {
					got, err := k8sCli.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
					if err != nil {
						return false, err
					}
					if got.Spec.NodeName == "" {
						return false, nil
					}
				}

				return true, nil
			})
			Expect(err).NotTo(HaveOccurred())

			// Make all the Pods once to prevent them from reaching PreFilter one by one before all the Pods in the PodGroup are created.
			for i := 0; i < 3; i++ {
				pod := MakePod(DefaultNs, fmt.Sprintf("throttled-pod%d", i), "100m").Annotation(groupNameAnnotation, "throttled").Label(throttleKey, throttleName).Obj()
				pod.Spec.SchedulingGates = []corev1.PodSchedulingGate{{Name: "group"}}
				pod = MustCreatePod(ctx, pod)
				podThrottledGroup = append(podThrottledGroup, pod)
			}

			for _, pod := range podThrottledGroup {
				applyConfig := corev1apply.Pod(pod.Name, pod.Namespace).WithSpec(corev1apply.PodSpec())
				applyConfig.Spec.SchedulingGates = nil
				_, err := k8sCli.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, k8stypes.StrategicMergePatchType, []byte(`{"spec":{"schedulingGates":null}}`), metav1.PatchOptions{})
				Expect(err).NotTo(HaveOccurred())
			}
		})
		It("should not schedule podThrottledGroup", func() {
			for _, pod := range podPassedGroup {
				Eventually(PodIsScheduled(ctx, DefaultNs, pod.Name)).Should(Succeed())
			}
			for _, pod := range podThrottledGroup {
				Eventually(MustPodFailedScheduling(ctx, DefaultNs, pod.Name, v1alpha1.CheckThrottleStatusInsufficientIncludingPodGroup)).Should(Succeed())
			}
			Eventually(AsyncAll(
				WakeupBackoffPod(ctx),
				ClusterThottleHasStatus(
					ctx, thr.Name,
					ClthrOpts.WithCalculatedThreshold(thr.Spec.Threshold),
					ClthrOpts.WithUsedPod(len(podPassedGroup)),
					ClthrOpts.WithUsedCpuReq(fmt.Sprintf("%dm", len(podPassedGroup)*100)),
					ClthrOpts.WithPodThrottled(false),
					ClthrOpts.WithCpuThrottled(false),
				),
			)).Should(Succeed())
			Consistently(func(g types.Gomega) {
				for _, pod := range podPassedGroup {
					PodIsScheduled(ctx, DefaultNs, pod.Name)
				}
			}).Should(Succeed())
		})
	})

	When("Many pods are created at once", func() {
		var thr *v1alpha1.ClusterThrottle
		var scheduled = make([]*corev1.Pod, 20)
		var pending *corev1.Pod
		BeforeEach(func() {
			thr = MustCreateClusterThrottle(ctx,
				MakeClusterThrottle(throttleName).Selector(DefaultNs, throttleKey, throttleName).
					ThresholdCpu("1").
					Obj(),
			)
			for i := range scheduled {
				scheduled[i] = MustCreatePod(ctx, MakePod(DefaultNs, fmt.Sprintf("pod-%d", i), "50m").Label(throttleKey, throttleName).Obj())
			}
			pending = MustCreatePod(ctx, MakePod(DefaultNs, "pod-20", "50m").Label(throttleKey, throttleName).Obj())
		})
		It("should throttle correctly", func() {
			Eventually(AsyncAll(
				WakeupBackoffPod(ctx),
				AsyncPods(scheduled, func(p *corev1.Pod) func(g Gomega) { return PodIsScheduled(ctx, DefaultNs, p.Name) }),
				ClusterThottleHasStatus(
					ctx, thr.Name,
					ClthrOpts.WithCalculatedThreshold(thr.Spec.Threshold),
					ClthrOpts.WithUsedPod(20), ClthrOpts.WithUsedCpuReq("1"),
					ClthrOpts.WithPodThrottled(false), ClthrOpts.WithCpuThrottled(true),
				),
				MustPodFailedScheduling(ctx, DefaultNs, pending.Name, v1alpha1.CheckThrottleStatusActive),
			)).Should(Succeed())
			Consistently(PodIsNotScheduled(ctx, DefaultNs, pending.Name)).Should(Succeed())
		})
	})
})
