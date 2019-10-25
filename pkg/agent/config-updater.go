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
	"context"
	"net"
	"time"

	"google.golang.org/grpc"

	resmgr_v1 "github.com/intel/cri-resource-manager/pkg/cri/resource-manager/config/api/v1"
	"github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// configuration update rate-limiting timeout
	rateLimitTimeout = 2 * time.Second
	// setConfigTimeout is the duration we wait at most for a SetConfig reply
	setConfigTimeout = 5 * time.Second
)

// configUpdater handles sending configuration to cri-resmgr
type configUpdater interface {
	Start() error
	Stop()
	Update(*resmgrConfig)
}

// updater implements configUpdater
type updater struct {
	log.Logger
	resmgrCli resmgr_v1.ConfigClient
	newConfig chan *resmgrConfig
}

func newConfigUpdater(socket string) (configUpdater, error) {
	u := &updater{Logger: log.NewLogger("config-updater")}

	c, err := newResmgrCli(opts.resmgrSocket)
	if err != nil {
		return nil, agentError("failed to create connection to cri-resmgr")
	}
	u.resmgrCli = c

	u.newConfig = make(chan *resmgrConfig)

	return u, nil

}

func (u *updater) Start() error {
	u.Info("Starting config-updater")
	go func() {
		var pending *resmgrConfig
		var ratelimit <-chan time.Time

		for {
			select {
			case cfg := <-u.newConfig:
				u.Info("scheduling update after %v rate-limiting timeout...", rateLimitTimeout)
				pending = cfg
				ratelimit = time.After(rateLimitTimeout)

			case _ = <-ratelimit:
				if _, err := u.setConfig(pending); err != nil {
					u.Error("failed to send configuration update: %v", err)
				} else {
					pending = nil
					ratelimit = nil
				}
			}
		}
	}()

	return nil
}

func (u *updater) Stop() {
}

func (u *updater) Update(c *resmgrConfig) {
	u.newConfig <- c
}

func (u *updater) setConfig(cfg *resmgrConfig) (*resmgr_v1.SetConfigReply, error) {
	ctx, cancel := context.WithTimeout(context.Background(), setConfigTimeout)
	defer cancel()

	req := &resmgr_v1.SetConfigRequest{NodeName: nodeName, Config: *cfg}
	u.Debug("sending SetConfig request to cri-resmgr")
	return u.resmgrCli.SetConfig(ctx, req, []grpc.CallOption{grpc.FailFast(false)}...)
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
