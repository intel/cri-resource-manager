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

package rdt

import (
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strconv"
	"strings"

	pkgcfg "github.com/intel/cri-resource-manager/pkg/config"
	"github.com/intel/cri-resource-manager/pkg/utils"
)

// options represents the raw RDT configuration data from the configmap
type options struct {
	Options    schemaOptions `json:"options"`
	Partitions map[string]struct {
		L3Allocation rawAllocations `json:"l3Allocation"`
		MBAllocation rawAllocations `json:"mbAllocation"`
		Classes      map[string]struct {
			L3Schema rawAllocations `json:"l3Schema"`
			MBSchema rawAllocations `json:"mbSchema"`
		} `json:"classes"`
	} `json:"partitions"`
}

// rawAllocations is an intermediate helper type for JSON parsing of
// per-cache-id allocations
type rawAllocations map[string]interface{}

// config represents the final (parsed and resolved) runtime configuration of
// RDT Control
type config struct {
	Options    schemaOptions
	Partitions map[string]partitionConfig
	Classes    map[string]classConfig
}

// partitionConfig is the final configuration of one partition
type partitionConfig struct {
	L3 map[uint64]Bitmask
	MB map[uint64]uint64
}

// classConfig represents configuration of one class, i.e. one CTRL group in
// the Linux resctrl interface
type classConfig struct {
	Partition string
	L3Schema  l3Schema
	MBSchema  mbSchema
}

// schemaOptions contains the common settings for all classes
type schemaOptions struct {
	L3 l3Options `json:"l3"`
	MB mbOptions `json:"mb"`
}

// l3Options contains the common settings for L3 cache allocation
type l3Options struct {
	Optional bool `json:"optional,omitempty"`
}

// mbOptions contains the common settings for memory bandwidth allocation
type mbOptions struct {
	Optional bool `json:"optional,omitempty"`
}

// l3Schema represents the L3 part of the schemata of a class (i.e. resctrl group)
type l3Schema map[uint64]l3Allocation

// mbSchema represents the MB part of the schemata of a class (i.e. resctrl group)
type mbSchema map[uint64]uint64

// l3Allocation describes the L3 allocation configuration for one cache id
type l3Allocation struct {
	Unified cacheAllocation
	Code    cacheAllocation
	Data    cacheAllocation
}

// cacheAllocation is the basic interface for handling cache allocations of one
// type (unified, code, data)
type cacheAllocation interface {
	Overlay(Bitmask) (Bitmask, error)
}

// l3AbsoluteAllocation represents an explicitly specified cache allocation
// bitmask
type l3AbsoluteAllocation Bitmask

// l3RelativeAllocation represents a percentage range of the available bitmask
type l3RelativeAllocation struct {
	lowPct  uint64
	highPct uint64
}

// L3SchemaType represents different L3 cache allocation schemes
type l3SchemaType string

const (
	// l3SchemaTypeUnified is the schema type when CDP is not enabled
	l3SchemaTypeUnified = ""
	// l3SchemaTypeCode is the 'code' part of CDP schema
	l3SchemaTypeCode = "CODE"
	// l3SchemaTypeData is the 'data' part of CDP schema
	l3SchemaTypeData = "DATA"
)

const (
	mbSuffixPct  = "%"
	mbSuffixMbps = "MBps"
)

// ToStr returns the L3 schema in a format accepted by the Linux kernel
// resctrl (schemata) interface
func (s l3Schema) ToStr(typ l3SchemaType, baseSchema map[uint64]Bitmask) (string, error) {
	schema := "L3" + string(typ) + ":"
	sep := ""

	for id, bitmask := range baseSchema {
		if s != nil {
			var err error

			masks := s[id]
			// Use Unified as the default/fallback for Code and Data
			overlayMask := masks.Unified
			switch typ {
			case l3SchemaTypeCode:
				if masks.Code != nil {
					overlayMask = masks.Code
				}
			case l3SchemaTypeData:
				if masks.Data != nil {
					overlayMask = masks.Data
				}
			}

			bitmask, err = overlayMask.Overlay(bitmask)
			if err != nil {
				return "", err
			}
		}
		schema += fmt.Sprintf("%s%d=%x", sep, id, bitmask)
		sep = ";"
	}

	return schema + "\n", nil
}

// Overlay function of the cacheAllocation interface
func (a l3AbsoluteAllocation) Overlay(baseMask Bitmask) (Bitmask, error) {
	shiftWidth := baseMask.lsbOne()
	if shiftWidth < 0 {
		return 0, rdtError("empty basemask not allowed")
	}

	// Treat our bitmask relative to the basemask
	bitmask := Bitmask(a) << shiftWidth

	// Do bounds checking that we're "inside" the base mask
	if bitmask|baseMask != baseMask {
		return 0, rdtError("bitmask %#x (%#x << %d) does not fit basemask %#x", bitmask, a, shiftWidth, baseMask)
	}

	return bitmask, nil
}

// MarshalJSON implements the Marshaler interface of "encoding/json"
func (a l3AbsoluteAllocation) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%#x\"", a)), nil
}

// Overlay function of the cacheAllocation interface
func (a l3RelativeAllocation) Overlay(baseMask Bitmask) (Bitmask, error) {
	baseMaskMsb := uint64(baseMask.msbOne())
	baseMaskLsb := uint64(baseMask.lsbOne())
	baseMaskNumBits := baseMaskMsb - baseMaskLsb + 1

	// Check that the basemask contains one (and only one) contiguous block of
	// (enough) bits set
	if bits.OnesCount64(uint64(baseMask)) != int(baseMaskNumBits) {
		return 0, rdtError("invalid basemask %#x: more than one block of bits set", baseMask)
	}
	if uint64(bits.OnesCount64(uint64(baseMask))) < rdtInfo.l3MinCbmBits() {
		return 0, rdtError("invalid basemask %#x: fewer than %d bits set", baseMask, rdtInfo.l3MinCbmBits())
	}

	low, high := a.lowPct, a.highPct
	if low == 0 {
		low = 1
	}
	if low > high || low > 100 || high > 100 {
		return 0, rdtError("invalid percentage range in %v", a)
	}

	// Convert percentage limits to bit numbers
	// Our effective range is 1%-100%, use substraction (-1) because of
	// arithmetics, so that we don't overflow on 100%
	lsb := (low - 1) * baseMaskNumBits / 100
	msb := (high - 1) * baseMaskNumBits / 100

	// Make sure the number of bits set satisfies the minimum requirement
	numBits := msb - lsb + 1
	if numBits < rdtInfo.l3MinCbmBits() {
		gap := rdtInfo.l3MinCbmBits() - numBits

		// First, widen the mask from the "lsb end"
		lsbAvailable := lsb - baseMaskLsb
		if gap <= lsbAvailable {
			lsb -= gap
		} else {
			lsb = baseMaskLsb
		}
		// If needed, widen the mask from the "msb end"
		numBits = msb - lsb + 1
		gap = rdtInfo.l3MinCbmBits() - numBits
		msbAvailable := baseMaskMsb - msb
		if gap <= msbAvailable {
			msb += gap
		} else {
			return 0, rdtError("BUG: not enough bits available for cache bitmask (%s applied on basemask %#x)", a, baseMask)
		}
	}

	value := ((1 << (msb - lsb + 1)) - 1) << (lsb + baseMaskLsb)

	return Bitmask(value), nil
}

// MarshalJSON implements the Marshaler interface of "encoding/json"
func (a l3RelativeAllocation) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%d-%d%%\"", a.lowPct, a.highPct)), nil
}

// ToStr returns the MB schema in a format accepted by the Linux kernel
// resctrl (schemata) interface
func (s mbSchema) ToStr(base map[uint64]uint64) string {
	schema := "MB:"
	sep := ""

	for id, baseAllocation := range base {
		value := uint64(0)
		if rdtInfo.mb.mbpsEnabled {
			value = math.MaxUint32
			if s != nil {
				value = s[id]
			}
			// Limit to given base value
			if value > baseAllocation {
				value = baseAllocation
			}
		} else {
			allocation := uint64(100)
			if s != nil {
				allocation = s[id]
			}
			value = allocation * baseAllocation / 100
			// Guarantee minimum bw so that writing out the schemata does not fail
			if value < rdtInfo.mb.minBandwidth {
				value = rdtInfo.mb.minBandwidth
			}
		}

		schema += fmt.Sprintf("%s%d=%d", sep, id, value)
		sep = ";"
	}

	return schema + "\n"
}

// listStrToArray parses a string containing a human-readable list of numbers
// into an integer array
func listStrToArray(str string) ([]int, error) {
	a := []int{}

	// Empty list
	if len(str) == 0 {
		return a, nil
	}

	ranges := strings.Split(str, ",")
	for _, ran := range ranges {
		split := strings.SplitN(ran, "-", 2)

		// We limit to 8 bits in order to avoid accidental super long slices
		num, err := strconv.ParseInt(split[0], 10, 8)
		if err != nil {
			return a, rdtError("invalid integer %q: %v", str, err)
		}

		if len(split) == 1 {
			a = append(a, int(num))
		} else {
			endNum, err := strconv.ParseInt(split[1], 10, 8)
			if err != nil {
				return a, rdtError("invalid integer in range %q: %v", str, err)
			}
			if endNum <= num {
				return a, rdtError("invalid integer range %q in %q", ran, str)
			}
			for i := num; i <= endNum; i++ {
				a = append(a, int(i))
			}
		}
	}
	sort.Ints(a)
	return a, nil
}

// resolve tries to resolve the requested configuration into a working
// configuration
func (raw options) resolve() (config, error) {
	var err error
	conf := config{Options: raw.Options}

	log.Debug("resolving configuration options:\n%s", utils.DumpJSON(raw))

	conf.Partitions, err = raw.resolvePartitions()
	if err != nil {
		return conf, err
	}

	conf.Classes, err = raw.resolveClasses()
	if err != nil {
		return conf, err
	}

	return conf, nil
}

// resolvePartitions tries to resolve the requested resource allocations of
// partitions
func (raw options) resolvePartitions() (map[string]partitionConfig, error) {
	// Initialize empty partition configuration
	conf := make(map[string]partitionConfig, len(raw.Partitions))
	numCacheIds := len(rdtInfo.cacheIds)
	for name := range raw.Partitions {
		conf[name] = partitionConfig{L3: make(map[uint64]Bitmask, numCacheIds),
			MB: make(map[uint64]uint64, numCacheIds)}
	}

	// Try to resolve L3 partition allocations
	err := raw.resolveL3Partitions(conf)
	if err != nil {
		return nil, err
	}

	// Try to resolve MB partition allocations
	err = raw.resolveMBPartitions(conf)
	if err != nil {
		return nil, err
	}

	return conf, nil
}

// resolveL3Partitions tries to resolve requested L3 allocations between partitions
func (raw options) resolveL3Partitions(conf map[string]partitionConfig) error {
	type partitionAllocation struct {
		name       string
		allocation uint64
	}

	cacheAllocations := map[uint64][]partitionAllocation{}
	for _, id := range rdtInfo.cacheIds {
		cacheAllocations[id] = make([]partitionAllocation, 0, len(raw.Partitions))
	}
	// Helper structure for printing out human-readable info in the end
	requests := map[string]map[uint64]uint64{}

	// Parse percentage values from raw config and transfer them to our
	// per-cache-id structure
	for name, partition := range raw.Partitions {
		allocations, err := partition.L3Allocation.parsePercentage()
		requests[name] = allocations
		if err != nil {
			return fmt.Errorf("failed to parse L3 allocation for partition %q: %v", name, err)
		}
		for id, val := range allocations {
			cacheAllocations[id] = append(cacheAllocations[id], partitionAllocation{name: name, allocation: val})
		}
	}

	// Next, try to resolve partition allocations, separately for each cache-id
	fullBitmaskNumBits := uint64(rdtInfo.l3CbmMask().lsbZero())
	for id, partitions := range cacheAllocations {
		// Sort partition allocations. We want to resolve smallest allocations
		// first in order to try to ensure that all allocations can be satisfied
		// because small percentages might need to be rounded up
		sort.Slice(partitions, func(i, j int) bool { return partitions[i].allocation < partitions[j].allocation })

		// Sanity check: check that total allocation requested for this cache
		// id does not exceed 100 percent
		total := uint64(0)
		for _, partition := range partitions {
			total += partition.allocation
		}
		if total < 100 {
			log.Info("requested total L3 partition allocation for cache id %d <100%% (%d%%)", id, total)
		} else if total > 100 {
			return fmt.Errorf("accumulated L3 partition allocation requests for cache id %d exceed 100%%", id)
		}

		bitID := uint64(0)
		minCbmBits := rdtInfo.l3MinCbmBits()
		for i, partition := range partitions {
			bitsAvailable := fullBitmaskNumBits - bitID
			percentageAvailable := bitsAvailable * 100 / fullBitmaskNumBits

			// This might happen e.g. if number of partitions would be greater
			// than the total number of bits
			if bitsAvailable < minCbmBits {
				return fmt.Errorf("unable to resolve L3 allocation for cache id %d, not enough exlusive bits available", id)
			}

			// Calculate number of bits allocated for this partition.
			// Use integer arithmetics, effectively always rounding down
			// fractional allocations i.e. trying to avoid over-allocation
			numBits := partition.allocation * bitsAvailable / percentageAvailable

			// Guarantee a non-zero allocation
			if numBits < minCbmBits {
				numBits = minCbmBits
			}
			// Don't overflow, allocate all remaining bits to the last partition
			if numBits > bitsAvailable || i == len(partitions)-1 {
				numBits = bitsAvailable
			}

			// Compose the actual bitmask
			conf[partition.name].L3[id] = Bitmask(((1 << numBits) - 1) << bitID)

			bitID += numBits
		}
	}

	log.Info("actual (and requested) L3 allocations per partition and cache id:")
	infoStr := ""
	for name, partition := range requests {
		infoStr += name + "\n    "
		for id, requestedPct := range partition {
			truePct := float64(bits.OnesCount64(uint64(conf[name].L3[id]))) * 100 / float64(fullBitmaskNumBits)
			requestedPctStr := fmt.Sprintf("(%d%%)", requestedPct)
			infoStr += fmt.Sprintf("%2d: %5.1f%% %-6s", id, truePct, requestedPctStr)
		}
		infoStr += "\n"
	}
	log.InfoBlock("    ", "%s", infoStr)

	return nil
}

// resolveMBPartitions tries to resolve requested MB allocations between partitions
func (raw options) resolveMBPartitions(conf map[string]partitionConfig) error {
	// We use percentage values directly from the raw conf
	for name, partition := range raw.Partitions {
		allocations, err := partition.MBAllocation.parseMB()
		if err != nil {
			return fmt.Errorf("failed to resolve MB allocation for partition %q: %v", name, err)
		}
		for id, allocation := range allocations {
			conf[name].MB[id] = allocation
			// Check that we don't go under the minimum allowed bandwidth setting
			if !rdtInfo.mb.mbpsEnabled && allocation < rdtInfo.mb.minBandwidth {
				conf[name].MB[id] = rdtInfo.mb.minBandwidth
			}
		}
	}

	return nil
}

// resolveClasses tries to resolve class allocations of all partitions
func (raw options) resolveClasses() (map[string]classConfig, error) {
	classes := make(map[string]classConfig)

	for bname, partition := range raw.Partitions {
		for gname, class := range partition.Classes {
			if _, ok := classes[gname]; ok {
				return classes, fmt.Errorf("class names must be unique, %q defined multiple times", gname)
			}

			var err error
			gc := classConfig{Partition: bname}

			gc.L3Schema, err = class.L3Schema.parseL3()
			if err != nil {
				return classes, fmt.Errorf("failed to resolve L3 allocation for class %q: %v", gname, err)
			}
			gc.MBSchema, err = class.MBSchema.parseMB()
			if err != nil {
				return classes, fmt.Errorf("failed to resolve MB allocation for class %q: %v", gname, err)
			}

			classes[gname] = gc
		}
	}

	return classes, nil
}

// parsePercentage parses a percentage value
func (raw rawAllocations) parsePercentage() (map[uint64]uint64, error) {
	rawValues, err := raw.rawParse("100%", true)
	if err != nil || rawValues == nil {
		return nil, err
	}

	allocations := make(map[uint64]uint64, len(rawValues))
	for id, rawVal := range rawValues {
		s, ok := rawVal.(string)
		if !ok {
			return nil, fmt.Errorf("not a string value %q", rawVal)
		}
		allocations[id], err = parsePercentage(s)
		if err != nil {
			return nil, err
		}
	}

	return allocations, nil
}

// parse parses a raw L3 cache allocation
func (raw rawAllocations) parseL3() (l3Schema, error) {
	rawValues, err := raw.rawParse("100%", false)
	if err != nil || rawValues == nil {
		return nil, err
	}

	allocations := make(l3Schema, len(rawValues))
	for id, rawVal := range rawValues {
		allocations[id], err = parseL3Allocation(rawVal)
		if err != nil {
			return nil, err
		}
	}

	return allocations, nil
}

// parseMB parses a raw MB allocation
func (raw rawAllocations) parseMB() (mbSchema, error) {
	rawValues, err := raw.rawParse(map[string]interface{}{}, false)
	if err != nil || rawValues == nil {
		return nil, err
	}

	allocations := make(mbSchema, len(rawValues))
	for id, rawVal := range rawValues {
		strList, ok := rawVal.([]interface{})
		if !ok {
			return nil, fmt.Errorf("not a string value %q", rawVal)
		}
		allocations[id], err = parseMBAllocation(strList)
		if err != nil {
			return nil, err
		}
	}

	return allocations, nil
}

// rawParse "pre-parses" the rawAllocations per each cache id. I.e. it assigns
// a raw (string) allocation for each cache id
func (raw rawAllocations) rawParse(defaultVal interface{}, initEmpty bool) (map[uint64]interface{}, error) {
	if raw == nil && !initEmpty {
		return nil, nil
	}

	if all, ok := raw["all"]; ok {
		defaultVal = all
	} else if defaultVal == nil {
		return nil, fmt.Errorf("'all' is missing")
	}

	allocations := make(map[uint64]interface{}, len(rdtInfo.cacheIds))
	for _, i := range rdtInfo.cacheIds {
		allocations[i] = defaultVal
	}

	for key, val := range raw {
		if key == "all" {
			continue
		}
		ids, err := listStrToArray(key)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := allocations[uint64(id)]; ok {
				allocations[uint64(id)] = val
			}
		}
	}

	return allocations, nil
}

// parsePercentage parses a percentage value from a string
func parsePercentage(s string) (uint64, error) {
	if s[len(s)-1] != '%' {
		return 0, fmt.Errorf("%q not a percentage value", s)
	}
	val, err := strconv.ParseUint(s[:len(s)-1], 10, 7)
	if err != nil {
		return 0, fmt.Errorf("invalid percentage value %q: %v", s, err)
	}
	if val < 1 || val > 100 {
		return 0, fmt.Errorf("percentage value %q out of range (1-100)", s)
	}
	return val, nil
}

// parseL3Allocation parses a generic string map into l3Allocation struct
func parseL3Allocation(raw interface{}) (l3Allocation, error) {
	var err error
	allocation := l3Allocation{}

	switch value := raw.(type) {
	case string:
		allocation.Unified, err = parseCacheAllocation(value)
		if err != nil {
			return allocation, err
		}
	case map[string]interface{}:
		for k, v := range value {
			s, ok := v.(string)
			if !ok {
				return allocation, fmt.Errorf("not a string value %q", v)
			}
			switch strings.ToLower(k) {
			case "unified":
				allocation.Unified, err = parseCacheAllocation(s)
			case "code":
				allocation.Code, err = parseCacheAllocation(s)
			case "data":
				allocation.Data, err = parseCacheAllocation(s)
			}
			if err != nil {
				return allocation, err
			}
		}
	default:
		return allocation, fmt.Errorf("invalid structure of l3Schema %q", raw)
	}

	// Sanity check for the configuration
	if allocation.Unified == nil {
		return allocation, fmt.Errorf("'unified' not specified in l3Schema %s", raw)
	}
	if allocation.Code != nil && allocation.Data == nil {
		return allocation, fmt.Errorf("'code' specified but missing 'data' from l3Schema %s", raw)
	}
	if allocation.Code == nil && allocation.Data != nil {
		return allocation, fmt.Errorf("'data' specified but missing 'code' from l3Schema %s", raw)
	}

	return allocation, nil
}

// parseCacheAllocation parses a string value into cacheAllocation type
func parseCacheAllocation(data string) (cacheAllocation, error) {
	if data[len(data)-1] == '%' {
		// Percentages of the max number of bits
		split := strings.SplitN(data[0:len(data)-1], "-", 2)
		var low, high uint64
		var err error
		var allocation l3RelativeAllocation

		if len(split) == 1 {
			high, err = strconv.ParseUint(split[0], 10, 7)
			if err != nil {
				return allocation, err
			}
		} else {
			low, err = strconv.ParseUint(split[0], 10, 7)
			if err != nil {
				return allocation, err
			}
			high, err = strconv.ParseUint(split[1], 10, 7)
			if err != nil {
				return allocation, err
			}
		}
		if low > high || low > 100 || high > 100 {
			return allocation, fmt.Errorf("invalid percentage range %q", data)
		}
		allocation = l3RelativeAllocation{lowPct: low, highPct: high}

		return allocation, nil
	}

	// Absolute allocation
	var value uint64
	var err error
	if strings.HasPrefix(data, "0x") {
		// Hex value
		value, err = strconv.ParseUint(data[2:], 16, 64)
		if err != nil {
			return nil, err
		}
	} else {
		// Last, try "list" format (i.e. smthg like 0,2,5-9,...)
		tmp, err := ListStrToBitmask(data)
		value = uint64(tmp)
		if err != nil {
			return nil, err
		}
	}

	// Sanity check of absolute allocation: bitmask must (only) contain one
	// contiguous block of ones wide enough
	numOnes := bits.OnesCount64(value)
	if numOnes != 64-bits.LeadingZeros64(value)-bits.TrailingZeros64(value) {
		return nil, fmt.Errorf("invalid cache bitmask %q: more than one continuous block of ones", data)
	}
	if uint64(numOnes) < rdtInfo.l3MinCbmBits() {
		return nil, fmt.Errorf("invalid cache bitmask %q: number of bits less than %d", data, rdtInfo.l3MinCbmBits())
	}

	return l3AbsoluteAllocation(value), nil
}

// parseMBAllocation parses a generic string map into MB allocation value
func parseMBAllocation(raw []interface{}) (uint64, error) {
	for _, v := range raw {
		strVal, ok := v.(string)
		if !ok {
			return 0, fmt.Errorf("not a string value %q", v)
		}
		if strings.HasSuffix(strVal, mbSuffixPct) {
			if !rdtInfo.mb.mbpsEnabled {
				value, err := strconv.ParseUint(strings.TrimSuffix(strVal, mbSuffixPct), 10, 7)
				if err != nil {
					return 0, err
				}
				return value, nil
			}
		} else if strings.HasSuffix(strVal, mbSuffixMbps) {
			if rdtInfo.mb.mbpsEnabled {
				value, err := strconv.ParseUint(strings.TrimSuffix(strVal, mbSuffixMbps), 10, 32)
				if err != nil {
					return 0, err
				}
				return value, nil
			}
		} else {
			log.Warn("unrecognized MBA allocation format in %q", strVal)
		}
	}

	// No value for the active mode was specified
	if rdtInfo.mb.mbpsEnabled {
		return 0, fmt.Errorf("missing 'MBps' value from mbSchema; required because 'mba_MBps' is enabled in the system")
	}
	return 0, fmt.Errorf("missing '%%' value from mbSchema; required because percentage-based MBA allocation is enabled in the system")
}

// Currently active set of "raw" options
var opt = defaultOptions().(*options)

// defaultOptions returns a new instance of "raw" options set to their defaults
func defaultOptions() interface{} {
	return &options{}
}

// Register us for configuration handling.
func init() {
	pkgcfg.Register("rdt", "RDT control", opt, defaultOptions)
}
