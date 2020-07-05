/*
Copyright 2019 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package agent

import (
	"fmt"
	"net"
	"time"

	"context"
	"encoding/json"

	"google.golang.org/grpc"

	resmgr "github.com/intel/cri-resource-manager/pkg/apis/resmgr/v1alpha1"
	resmgr_v1 "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config/api/v1"
	"github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// configuration update rate-limiting timeout
	rateLimitTimeout = 2 * time.Second
	// setConfigTimeout is the duration we wait at most for a SetConfig reply
	setConfigTimeout = 5 * time.Second
	// retryTimeout is the timeout after we retry sending configuration updates upon failure
	retryTimeout = 5 * time.Second
)

// configUpdater handles sending configuration to cri-resmgr
type configUpdater interface {
	Start() error
	Stop()
	UpdateConfig(*resmgrConfig)
	UpdateAdjustment(*resmgrAdjustment)
	StatusChan() chan *resmgrStatus
}

// updater implements configUpdater
type updater struct {
	log.Logger
	resmgrCli     resmgr_v1.ConfigClient
	newConfig     chan *resmgrConfig
	newAdjustment chan *resmgrAdjustment
	newStatus     chan *resmgrStatus
}

func newConfigUpdater(socket string) (configUpdater, error) {
	u := &updater{Logger: log.NewLogger("config-updater")}

	c, err := newResmgrCli(opts.resmgrSocket)
	if err != nil {
		return nil, agentError("failed to create connection to cri-resmgr")
	}
	u.resmgrCli = c

	u.newConfig = make(chan *resmgrConfig)
	u.newAdjustment = make(chan *resmgrAdjustment)
	u.newStatus = make(chan *resmgrStatus)

	return u, nil
}

func (u *updater) Start() error {
	u.Info("Starting config-updater")
	go func() {
		var pendingConfig *resmgrConfig
		var pendingAdjustment *resmgrAdjustment

		var ratelimit <-chan time.Time

		for {
			select {
			case cfg := <-u.newConfig:
				u.Info("scheduling update after %v rate-limiting timeout...", rateLimitTimeout)
				pendingConfig = cfg
				ratelimit = time.After(rateLimitTimeout)

			case adjust := <-u.newAdjustment:
				u.Info("scheduling update after %v rate-limiting timeout...", rateLimitTimeout)
				pendingAdjustment = adjust
				ratelimit = time.After(rateLimitTimeout)

			case _ = <-ratelimit:
				if pendingConfig != nil {
					mgrErr, err := u.setConfig(pendingConfig)
					if err != nil {
						u.Error("failed to send configuration update: %v", err)
						ratelimit = time.After(retryTimeout)
					} else {
						if mgrErr != nil {
							u.Error("cri-resmgr configuration error: %v", mgrErr)
						}
						pendingConfig = nil
						ratelimit = nil
					}
				}
				if pendingAdjustment != nil {
					errors, err := u.setAdjustment(pendingAdjustment)

					if err != nil {
						u.Error("failed to update adjustments: %+v", err)
					}
					if len(errors) > 0 {
						u.Error("some adjustment updates failed: %+v", errors)
					}

					u.newStatus <- &resmgrStatus{
						request: err,
						errors:  errors,
					}

					pendingAdjustment = nil
					ratelimit = nil
				}
			}
		}
	}()

	return nil
}

func (u *updater) Stop() {
}

func (u *updater) UpdateConfig(c *resmgrConfig) {
	u.newConfig <- c
}

func (u *updater) UpdateAdjustment(c *resmgrAdjustment) {
	u.newAdjustment <- c
}

func (u *updater) StatusChan() chan *resmgrStatus {
	return u.newStatus
}

func (u *updater) setConfig(cfg *resmgrConfig) (error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), setConfigTimeout)
	defer cancel()

	req := &resmgr_v1.SetConfigRequest{NodeName: nodeName, Config: *cfg}
	u.Debug("sending SetConfig request to cri-resmgr")

	reply, err := u.resmgrCli.SetConfig(ctx, req, []grpc.CallOption{grpc.FailFast(false)}...)

	switch {
	case err != nil:
		return nil, err
	case reply.Error != "":
		return fmt.Errorf("%s", reply.Error), nil
	default:
		return nil, nil
	}
}

func (u *updater) setAdjustment(adjust *resmgrAdjustment) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), setConfigTimeout)
	defer cancel()

	specs := map[string]*resmgr.AdjustmentSpec{}
	for name, p := range *adjust {
		specs[name] = &resmgr.AdjustmentSpec{
			Scope:        p.Spec.NodeScope(nodeName),
			Resources:    p.Spec.Resources,
			Classes:      p.Spec.Classes,
			ToptierLimit: p.Spec.ToptierLimit,
		}
	}
	encoded, err := json.Marshal(specs)
	if err != nil {
		return nil, agentError("setAdjustment: failed to encode AdjustmentSpec: %v", err)
	}

	req := &resmgr_v1.SetAdjustmentRequest{NodeName: nodeName, Adjustment: string(encoded)}
	u.Debug("sending SetAdjustment request to cri-resmgr")

	reply, err := u.resmgrCli.SetAdjustment(ctx, req, []grpc.CallOption{grpc.FailFast(false)}...)

	if err != nil {
		return nil, err
	}
	return reply.Errors, nil
}

func newResmgrCli(socket string) (resmgr_v1.ConfigClient, error) {
	dialOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithDialer(func(sock string, timeout time.Duration) (net.Conn, error) {
			return net.Dial("unix", socket)
		}),
	}
	conn, err := grpc.Dial(socket, dialOpts...)
	if err != nil {
		return nil, agentError("failed to connect to cri-resmgr: %v", err)
	}
	return resmgr_v1.NewConfigClient(conn), nil
}
