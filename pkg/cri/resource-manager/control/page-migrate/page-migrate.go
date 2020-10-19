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
	cache      cache.Cache           // resource manager cache
	containers map[string]*container // containers we migrate
}

//
// The resource manager serializes access to the cache during request
// processing, event processing, and configuration updates by locking
// the resource-manager for each of these. Since controller hooks are
// invoked either as part of processing a request or an event, access
// to the cache from hooks is properly serialized.
//
// Page scanning or migration on the other hand happen asynchronously
// from dedicated goroutines. In order to avoid having to serialize
// access to the cache for these, we track and cache locally just enough
// data about containers that we can perform these actions completely on
// our own, without the need to access the resource manager cache at all.
//
// An alternative would have been to duplicate what we had originally in
// the policy:
//  - introduce controller events akin to policy events
//  - have the resource-manager call controller event handlers with the
//    lock held
//  - periodically inject a controller event when we want to scan pages
//  - perform page scanning or demotion from the event handler with the
//    resource-manager lock held
//
// However that would have destroyed one of the goals of splitting page
// scanning and migration out to a controller of its own, which was to
// perform these potentially time consuming actions without blocking
// concurrent processing of requests or events.
//

// container is the per container data we track locally.
type container struct {
	cacheID    string
	ID         string
	prettyName string
	parentDir  string
	pm         *cache.PageMigrate
}

// Our logger instance.
var log = logger.NewLogger(PageMigrationController)

// Our singleton page migration controller.
var singleton *migration

// getMigrationController returns our singleton controller instance.
func getMigrationController() *migration {
	if singleton == nil {
		singleton = &migration{
			containers: make(map[string]*container),
		}
	}
	return singleton
}

// Start prepares the controller for resource control/decision enforcement.
func (m *migration) Start(cache cache.Cache, client client.Client) error {
	m.cache = cache
	m.syncWithCache()
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
	err := m.insertContainer(cc)
	cc.ClearPending(PageMigrationController)
	return err
}

// PostUpdateHook is the controller's post-update hook.
func (m *migration) PostUpdateHook(cc cache.Container) error {
	m.updateContainer(cc)
	cc.ClearPending(PageMigrationController)
	return nil
}

// PostStopHook is the controller's post-stop hook.
func (m *migration) PostStopHook(cc cache.Container) error {
	m.deleteContainer(cc)
	return nil
}

// syncWithCache synchronizes tracked containers with the cache.
func (m *migration) syncWithCache() {
	m.Lock()
	defer m.Unlock()
	m.containers = make(map[string]*container)
	for _, cc := range m.cache.GetContainers() {
		m.insertContainer(cc)
	}
}

// insertContainer creates a local copy of the container.
func (m *migration) insertContainer(cc cache.Container) error {
	pm := cc.GetPageMigration()
	if pm == nil {
		return nil
	}

	pod, ok := cc.GetPod()
	if !ok {
		return migrationError("can't find pod for container %s",
			cc.PrettyName())
	}

	c := &container{
		cacheID:    cc.GetCacheID(),
		ID:         cc.GetID(),
		prettyName: cc.PrettyName(),
		parentDir:  pod.GetCgroupParentDir(),
		pm:         pm.Clone(),
	}
	if c.parentDir == "" {
		return migrationError("can't find cgroup parent dir for container %s",
			c.prettyName)
	}

	m.containers[c.cacheID] = c

	return nil
}

// updateContainer updates the local copy of the container.
func (m *migration) updateContainer(cc cache.Container) error {
	pm := cc.GetPageMigration()
	if pm == nil {
		delete(m.containers, cc.GetCacheID())
		return nil
	}

	c, ok := m.containers[cc.GetCacheID()]
	if !ok {
		return m.insertContainer(cc)
	}

	c.pm = pm.Clone()
	return nil
}

// deleteContainer creates a local copy of the container.
func (m *migration) deleteContainer(cc cache.Container) error {
	delete(m.containers, cc.GetCacheID())
	return nil
}

// GetCacheID replicates the respective cache.Container function.
func (c *container) GetCacheID() string {
	return c.cacheID
}

// GetID replicates the respective cache.Container function.
func (c *container) GetID() string {
	return c.id
}

// GetCgroupParentDir replicates the respective cache.Pod function.
func (c *container) GetCgroupParentDir() string {
	return c.parentDir
}

// GetPageMigration replicates the respective cache.Container function.
func (c *container) GetPageMigration() *cache.PageMigrate {
	return c.pm
}

// PrettyName replicates the respective cache.Container function.
func (c *container) PrettyName() string {
	return c.prettyName
}

// init registers this controller.
func init() {
	control.Register(PageMigrationController, "page migration controller", getMigrationController())
}

// migrationError creates a controller-specific formatted error message.
func migrationError(format string, args ...interface{}) error {
	return fmt.Errorf("page-migrate: "+format, args...)
}
