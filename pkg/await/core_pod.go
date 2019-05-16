package await

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-kubernetes/pkg/clients"
	"github.com/pulumi/pulumi-kubernetes/pkg/kinds"
	"github.com/pulumi/pulumi-kubernetes/pkg/logging"
	"github.com/pulumi/pulumi-kubernetes/pkg/metadata"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
)

// ------------------------------------------------------------------------------------------------

// Await logic for core/v1/Pod.
//
// Unlike the goals for the other "complex" awaiters, the goal of this code is to provide a
// fine-grained account of the status of a Kubernetes Pod as it is being initialized in the context
// of some controller (e.g., a Deployment, etc.).
//
// In our context (i.e., supporting `apps/v1*/Deployment`) this means our success measurement for
// Pods essentially boils down to:
//
//   * Waiting until `.status.phase` is set to "Running".
//   * Waiting until `.status.conditions` has a `Ready` condition, with `status` set to "Ready".
//
// But there are subtleties to this, and it's important to understand (1) the weaknesses of the Pod
// API, and (2) what impact they have on our ability to provide a compelling user experience for
// them.
//
// First, it is important to realize that aside from being explicitly designed to be flexible enough
// to be managed by a variety of controllers (e.g., Deployments, DaemonSets, ReplicaSets, and so
// on), they are nearly unusable on their own:
//
//   1. Pods are extremely difficult to manage: Once scheduled, they are bound to a node forever,
//      so if a node fails, the Pod is never rescheduled; there is no built-in, reliable way to
//      upgrade or change them with predictable consequences; and most importantly, there is no
//      advantage to NOT simply using a controller to manage them.
//   2. It is impossible to tell from the resource JSON schema alone whether a Pod is meant to run
//      indefinitely, or to terminate, which makes it hard to tell in general whether a deployment
//      was successful. These semantics are typically conferred by a controller that manages Pod
//      objects -- for example, a Deployment expects Pods to run indefinitely, while a Job expects
//      them to terminate.
//
// For each of these different controllers, there are different success conditions. For a
// Deployment, a Pod becomes successfully initialized when `.status.phase` is set to "Running". For
// a Job, a Pod becomes successful when it successfully completes. Since at this point we only
// support Deployment, we'll settle for the former as "the success condition" for Pods.
//
// The subtlety of this that "Running" actually just means that the Pod has been bound to a node,
// all containers have been created, and at least one is still "alive" -- a status is set once the
// liveness and readiness probes (which usually simply ping some endpoint, and which are
// customizable by the user) return success. Along the say several things can go wrong (e.g., image
// pull error, container exits with code 1), but each of these things would prevent the probes from
// reporting success, or they would be picked up by the Kubelet.
//
// The design of this awaiter is relatively simple, since the conditions of success are relatively
// straightforward. This awaiter relies on three channels:
//
//   1. The Pod channel, to which the Kubernetes API server will proactively push every change
//      (additions, modifications, deletions) to any Pod it knows about.
//   2. A timeout channel, which fires after some minutes.
//   3. A cancellation channel, with which the user can signal cancellation (e.g., using SIGINT).
//
// The `podInitAwaiter` will synchronously process events from the union of all these channels. Any
// time the success conditions described above a reached, we will terminate the awaiter.
//
// The opportunity to display intermediate results will typically appear after a container in the
// Pod fails, (e.g., volume fails to mount, image fails to pull, exited with code 1, etc.).
//
//
// x-refs:
//   * https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/

// ------------------------------------------------------------------------------------------------

// --------------------------------------------------------------------------

// POD CHECKING. Routines for checking whether a Pod has been initialized correctly.

// --------------------------------------------------------------------------

type PodState struct {
	ready      bool
	conditions []Condition
}

type Result struct {
	Ok          bool
	Description string
	Message     logging.Message
}

func toPod(obj interface{}) *corev1.Pod {
	return obj.(*corev1.Pod)
}

func (s *PodState) Update(pod *corev1.Pod) logging.Messages {
	s.ready = false

	var messages logging.Messages
	for i, condition := range s.conditions {
		prefix := fmt.Sprintf("[%d/%d]", i, len(s.conditions))

		result := condition(pod)
		messages = append(messages, logging.StatusMessage(fmt.Sprintf("%s %s", prefix, result.Description)))

		if !result.Ok {
			if !result.Message.Empty() {
				messages = append(messages, result.Message)
			}
			return messages
		}
	}

	s.ready = true
	messages = append(messages, logging.StatusMessage(fmt.Sprint("✅ Pod ready")))
	return messages
}

func (s *PodState) Ready() bool {
	return s.ready
}

func NewPodState(conditions ...Condition) *PodState {
	return &PodState{ready: false, conditions: conditions}
}

func NewPodChecker() *PodState {
	return NewPodState(podScheduled, podInitialized, podReady)
}

func podScheduled(obj interface{}) Result {
	pod := toPod(obj)
	result := Result{Description: fmt.Sprintf("Waiting for Pod %q to be scheduled", fqName(pod))}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled {
			if condition.Status == corev1.ConditionTrue {
				result.Ok = true
				return result
			}

			// TODO: this doesn't make sense
			result.Message = logging.StatusMessage(statusFromCondition(condition))
			return result
		}
	}

	return result
}

func podInitialized(obj interface{}) Result {
	pod := toPod(obj)
	result := Result{Description: fmt.Sprintf("Waiting for Pod %q to be initialized", fqName(pod))}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodInitialized {
			if condition.Status == corev1.ConditionTrue {
				result.Ok = true
				return result
			}

			var errs []string
			for _, status := range pod.Status.ContainerStatuses {
				if ok, containerErrs := hasContainerStatusErrors(status); !ok {
					errs = append(errs, containerErrs...)
				}
			}

			result.Message = logging.WarningMessage(podError(condition, errs))
			return result
		}
	}

	return result
}

func podReady(obj interface{}) Result {
	pod := toPod(obj)
	result := Result{Description: fmt.Sprintf("Waiting for Pod %q to be ready", fqName(pod))}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			if condition.Status == corev1.ConditionTrue {
				result.Ok = true
				return result
			}

			var errs []string
			for _, status := range pod.Status.ContainerStatuses {
				if hasErr, containerErrs := hasContainerStatusErrors(status); hasErr {
					errs = append(errs, containerErrs...)
				}
			}

			result.Message = logging.WarningMessage(podError(condition, errs))
			return result
		}
	}

	return result
}

func hasContainerStatusErrors(status corev1.ContainerStatus) (bool, []string) {
	if status.Ready {
		return false, nil
	}

	var errs []string
	if hasErr, err := hasContainerWaitingError(status); hasErr {
		errs = append(errs, err)
	}
	if hasErr, err := hasContainerTerminatedError(status); hasErr {
		errs = append(errs, err)
	}

	return len(errs) > 0, errs
}

func hasContainerWaitingError(status corev1.ContainerStatus) (bool, string) {
	state := status.State.Waiting
	if state == nil {
		return false, ""
	}

	// Return false if the container is creating.
	if state.Reason == "ContainerCreating" {
		return false, ""
	}

	// Image pull error has a bunch of useless junk in the error message. Try to remove it.
	trimPrefix := "rpc error: code = Unknown desc = Error response from daemon: "
	trimSuffix := ": manifest unknown"
	msg := state.Message
	msg = strings.TrimPrefix(msg, trimPrefix)
	msg = strings.TrimSuffix(msg, trimSuffix)

	return true, msg
}

func hasContainerTerminatedError(status corev1.ContainerStatus) (bool, string) {
	state := status.State.Terminated
	if state == nil {
		return false, ""
	}

	// Return false if no reason given.
	if len(state.Reason) == 0 {
		return false, ""
	}

	if len(state.Message) > 0 {
		return true, state.Message
	}
	return true, fmt.Sprintf("Container %q completed with exit code %d", status.Name, state.ExitCode)
}

func statusFromCondition(condition corev1.PodCondition) string {
	if condition.Reason != "" && condition.Message != "" {
		return condition.Message
	}

	return ""
}

func podError(condition corev1.PodCondition, errs []string) string {
	var errMsg string
	if len(condition.Reason) > 0 && len(condition.Message) > 0 {
		errMsg = condition.Message
	}

	for _, err := range errs {
		errMsg += fmt.Sprintf(" -- %s", err)
	}

	return errMsg
}

// --------------------------------------------------------------------------

// POD AWAITING. Routines for waiting until a Pod has been initialized correctly.

// --------------------------------------------------------------------------

type podInitAwaiter struct {
	pod      *unstructured.Unstructured
	config   createAwaitConfig
	state    *PodState
	messages logging.Messages
}

func makePodInitAwaiter(c createAwaitConfig) *podInitAwaiter {
	return &podInitAwaiter{
		config: c,
		pod:    c.currentOutputs,
		state:  NewPodState(podScheduled, podInitialized, podReady),
	}
}

func (pia *podInitAwaiter) errorMessages() []string {
	var messages []string
	for _, message := range pia.messages.Warnings() {
		messages = append(messages, message.S)
	}
	for _, message := range pia.messages.Errors() {
		messages = append(messages, message.S)
	}

	return messages
}

func awaitPodInit(c createAwaitConfig) error {
	return makePodInitAwaiter(c).Await()
}

func awaitPodRead(c createAwaitConfig) error {
	return makePodInitAwaiter(c).Read()
}

func awaitPodUpdate(u updateAwaitConfig) error {
	return makePodInitAwaiter(u.createAwaitConfig).Await()
}

func (pia *podInitAwaiter) Await() error {
	podClient, err := clients.ResourceClient(
		kinds.Pod, pia.config.currentInputs.GetNamespace(), pia.config.clientSet)
	if err != nil {
		return errors.Wrapf(err,
			"Could not make client to watch Pod %q",
			pia.config.currentInputs.GetName())
	}
	podWatcher, err := podClient.Watch(metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "Couldn't set up watch for Pod object %q",
			pia.config.currentInputs.GetName())
	}
	defer podWatcher.Stop()

	timeout := time.Duration(metadata.TimeoutSeconds(pia.config.currentInputs, 5*60)) * time.Second
	return pia.await(podWatcher, time.After(timeout))
}

func (pia *podInitAwaiter) Read() error {
	podClient, err := clients.ResourceClient(
		kinds.Pod, pia.config.currentInputs.GetNamespace(), pia.config.clientSet)
	if err != nil {
		return errors.Wrapf(err,
			"Could not make client to get Pod %q",
			pia.config.currentInputs.GetName())
	}
	// Get live version of Pod.
	pod, err := podClient.Get(pia.config.currentInputs.GetName(), metav1.GetOptions{})
	if err != nil {
		// IMPORTANT: Do not wrap this error! If this is a 404, the provider need to know so that it
		// can mark the Pod as having been deleted.
		return err
	}

	return pia.read(pod)
}

func (pia *podInitAwaiter) read(pod *unstructured.Unstructured) error {
	pia.processPodEvent(watchAddedEvent(pod))

	// Check whether we've succeeded.
	if pia.state.Ready() {
		return nil
	}

	return &initializationError{
		subErrors: pia.errorMessages(),
		object:    pod,
	}
}

// await is a helper companion to `Await` designed to make it easy to test this module.
func (pia *podInitAwaiter) await(podWatcher watch.Interface, timeout <-chan time.Time) error {
	for {
		if pia.state.Ready() {
			return nil
		}

		// Else, wait for updates.
		select {
		// TODO: If Pod is added and not making progress on initialization after ~30 seconds, report that.
		case <-pia.config.ctx.Done():
			return &cancellationError{
				object:    pia.pod,
				subErrors: pia.errorMessages(),
			}
		case <-timeout:
			return &timeoutError{
				object:    pia.pod,
				subErrors: pia.errorMessages(),
			}
		case event := <-podWatcher.ResultChan():
			pia.processPodEvent(event)
		}
	}
}

func (pia *podInitAwaiter) processPodEvent(event watch.Event) {
	pod, err := clients.PodFromUnstructured(event.Object.(*unstructured.Unstructured))
	if err != nil {
		glog.V(3).Infof("Failed to unmarshal Pod event: %v", err)
		return
	}

	// Do nothing if this is not the pod we're waiting for.
	if pod.GetName() != pia.config.currentInputs.GetName() {
		return
	}

	messages := pia.state.Update(pod)
	for _, message := range messages {
		pia.config.logMessage(message)
	}
}
