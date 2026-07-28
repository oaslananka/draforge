// Package main coordinates the controller's single-active lifecycle.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/oaslananka/draforge/internal/simulator"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
)

const (
	defaultLeaderLeaseName      = "draforge-controller"
	defaultLeaderLeaseNamespace = "default"
	defaultLeaderLeaseDuration  = 15 * time.Second
	defaultLeaderRenewDeadline  = 10 * time.Second
	defaultLeaderRetryPeriod    = 2 * time.Second
)

type controllerActiveFunc func(context.Context)

type leaderElectionOptions struct {
	Enabled        bool
	LeaseName      string
	LeaseNamespace string
	Identity       string
	LeaseDuration  time.Duration
	RenewDeadline  time.Duration
	RetryPeriod    time.Duration
}

func (options leaderElectionOptions) validate() error {
	if !options.Enabled {
		return nil
	}
	if errors := validation.IsDNS1123Subdomain(options.LeaseName); len(errors) > 0 {
		return fmt.Errorf("invalid leader-election lease name %q: %s", options.LeaseName, errors[0])
	}
	if errors := validation.IsDNS1123Label(options.LeaseNamespace); len(errors) > 0 {
		return fmt.Errorf("invalid leader-election lease namespace %q: %s", options.LeaseNamespace, errors[0])
	}
	if options.Identity == "" {
		return fmt.Errorf("leader-election identity must not be empty")
	}
	if len(options.Identity) > 128 {
		return fmt.Errorf("leader-election identity must not exceed 128 characters")
	}
	if options.LeaseDuration < time.Second {
		return fmt.Errorf("leader-election lease duration must be at least one second")
	}
	if options.RenewDeadline <= 0 {
		return fmt.Errorf("leader-election renew deadline must be positive")
	}
	if options.RetryPeriod <= 0 {
		return fmt.Errorf("leader-election retry period must be positive")
	}
	if options.LeaseDuration <= options.RenewDeadline {
		return fmt.Errorf("leader-election lease duration must be greater than renew deadline")
	}
	if options.RenewDeadline <= time.Duration(leaderelection.JitterFactor*float64(options.RetryPeriod)) {
		return fmt.Errorf("leader-election renew deadline must be greater than retry period multiplied by %.1f", leaderelection.JitterFactor)
	}
	return nil
}

func resolveLeaderElectionOptions(
	options leaderElectionOptions,
	getenv func(string) string,
	hostname func() (string, error),
) (leaderElectionOptions, error) {
	if !options.Enabled {
		return options, nil
	}
	if options.LeaseName == "" {
		options.LeaseName = defaultLeaderLeaseName
	}
	if options.LeaseNamespace == "" {
		options.LeaseNamespace = getenv("POD_NAMESPACE")
		if options.LeaseNamespace == "" {
			options.LeaseNamespace = defaultLeaderLeaseNamespace
		}
	}
	if options.Identity == "" {
		options.Identity = getenv("POD_NAME")
		if options.Identity == "" {
			resolvedHostname, err := hostname()
			if err != nil {
				return leaderElectionOptions{}, fmt.Errorf("resolve leader-election identity: %w", err)
			}
			options.Identity = resolvedHostname
		}
	}
	if options.LeaseDuration == 0 {
		options.LeaseDuration = defaultLeaderLeaseDuration
	}
	if options.RenewDeadline == 0 {
		options.RenewDeadline = defaultLeaderRenewDeadline
	}
	if options.RetryPeriod == 0 {
		options.RetryPeriod = defaultLeaderRetryPeriod
	}
	if err := options.validate(); err != nil {
		return leaderElectionOptions{}, err
	}
	return options, nil
}

func resolveProcessLeaderElectionOptions(options leaderElectionOptions) (leaderElectionOptions, error) {
	return resolveLeaderElectionOptions(options, os.Getenv, os.Hostname)
}

func runControllerLifecycle(
	ctx context.Context,
	clientset kubernetes.Interface,
	options leaderElectionOptions,
	reconciler *simulator.Reconciler,
	active controllerActiveFunc,
) error {
	if err := options.validate(); err != nil {
		return err
	}
	if active == nil {
		return fmt.Errorf("active controller callback must not be nil")
	}
	if !options.Enabled {
		reconciler.SetLeader(true)
		defer reconciler.SetLeader(false)
		active(ctx)
		return nil
	}
	if clientset == nil {
		return fmt.Errorf("leader election requires a Kubernetes client")
	}

	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		options.LeaseNamespace,
		options.LeaseName,
		clientset.CoreV1(),
		clientset.CoordinationV1(),
		resourcelock.ResourceLockConfig{Identity: options.Identity},
	)
	if err != nil {
		return fmt.Errorf("create leader-election lease lock: %w", err)
	}

	// state is 0 while the callback may start, 1 while active, and 2 once stopped.
	// The CAS prevents a delayed callback from starting after Run has returned.
	var state atomic.Int32
	activeDone := make(chan struct{})
	elector, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: options.LeaseDuration,
		RenewDeadline: options.RenewDeadline,
		RetryPeriod:   options.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(activeContext context.Context) {
				if !state.CompareAndSwap(0, 1) {
					return
				}
				reconciler.SetLeader(true)
				defer func() {
					reconciler.SetLeader(false)
					state.Store(2)
					close(activeDone)
				}()
				active(activeContext)
			},
			OnStoppedLeading: func() {
				klog.Infof("Controller %q stopped leading lease %s/%s", options.Identity, options.LeaseNamespace, options.LeaseName)
			},
			OnNewLeader: func(identity string) {
				klog.Infof("Observed controller leader %q for lease %s/%s", identity, options.LeaseNamespace, options.LeaseName)
			},
		},
		ReleaseOnCancel: false,
		Name:            options.LeaseName,
	})
	if err != nil {
		return fmt.Errorf("create leader elector: %w", err)
	}

	elector.Run(ctx)
	if state.CompareAndSwap(0, 2) {
		return nil
	}
	if state.Load() == 1 {
		<-activeDone
	}
	return nil
}
