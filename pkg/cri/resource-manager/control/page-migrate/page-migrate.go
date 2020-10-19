// Copyright 2019-2020 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pagemigrate

import (
	"fmt"

	"github.com/intel/cri-resource-manager/pkg/cri/client"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/cache"
	"github.com/intel/cri-resource-manager/pkg/cri/resource-manager/control"
	logger "github.com/intel/cri-resource-manager/pkg/log"
)

const (
	// PageMigrationController is the name/domain of the page migration controller.
	PageMigrationController = cache.PageMigration
	// PageMigrationConfigPath is the configuration path for the page migration controller.
	PageMigrationConfigPath = "resource-manager.control." + PageMigrationController
	// PageMigrationDescription is the description for the page migration controller.
	PageMigrationDescription = "page migration controller"
)

// migration implements the controller for memory page migration.
type migration struct {
	cache cache.Cache // resource manager cache
}

// Our logger instance.
var log = logger.NewLogger(PageMigrationController)

// Our singleton page migration controller.
var singleton *migration

// getMigrationController returns our singleton controller instance.
func getMigrationController() *migration {
	if singleton == nil {
		singleton = &migration{}
	}
	return singleton
}

// Start prepares the controller for resource control/decision enforcement.
func (m *migration) Start(cache cache.Cache, client client.Client) error {
	m.cache = cache
	return nil
}

// Stop shuts down the controller.
func (m *migration) Stop() {
}

// PreCreateHook is the controller's pre-create hook.
func (m *migration) PreCreateHook(cache.Container) error {
	return nil
}

// PreStartHook is the controller's pre-start hook.
func (m *migration) PreStartHook(cache.Container) error {
	return nil
}

// PostStartHook is the controller's post-start hook.
func (m *migration) PostStartHook(cc cache.Container) error {
	cc.ClearPending(PageMigrationController)
	return nil
}

// PostUpdateHook is the controller's post-update hook.
func (m *migration) PostUpdateHook(cc cache.Container) error {
	cc.ClearPending(PageMigrationController)
	return nil
}

// PostStopHook is the controller's post-stop hook.
func (m *migration) PostStopHook(cc cache.Container) error {
	return nil
}

// migrationError creates a controller-specific formatted error message.
func migrationError(format string, args ...interface{}) error {
	return fmt.Errorf("page-migrate: "+format, args...)
}

// init registers this controller.
func init() {
	control.Register(PageMigrationController, "page migration controller", getMigrationController())
}
