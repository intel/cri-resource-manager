// Copyright 2020 Intel Corporation. All Rights Reserved.
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
	idset "github.com/intel/goresctrl/pkg/utils"
	"testing"
)

type mockPageMover struct {
	firstSuccess               bool
	secondSuccess              bool
	expectedPagesForSecondCall uint
	firstStatus                []int
}

func (m *mockPageMover) MovePagesSyscall(pid int, count uint, pages []uintptr, nodes []int, flags int) (uint, []int, error) {

	status := make([]int, len(pages))

	fmt.Printf("move_pages(): pid %d, count %d, pages %v, nodes %v, flags %d\n",
		pid, count, pages, nodes, flags)

	if nodes == nil {
		// First call is made without nodes
		if m.firstSuccess == false {
			return 0, m.firstStatus, fmt.Errorf("Fake error")
		}
		return 0, m.firstStatus, nil
	}

	// Second call
	if m.secondSuccess == false {
		return 0, status, fmt.Errorf("Fake error")
	}
	if uint(len(pages)) != m.expectedPagesForSecondCall {
		return 0, status, fmt.Errorf("Real error")
	}

	return 0, status, nil
}

func TestMovePages(t *testing.T) {
	tcases := []struct {
		name                       string
		pool                       pagePool
		targetNodes                idset.IDSet
		pageCount                  uint
		expectedRemainingPageCount uint
		expectedError              bool
		pageMover                  PageMover
		pid                        int
	}{
		{
			name: "move pages (both)",
			pool: pagePool{
				pages: map[int][]page{
					500: {
						{
							pid:  500,
							addr: 0xdeadbeef,
						},
						{
							pid:  500,
							addr: 0xc0ffee,
						},
					},
				},
			},
			pid:       500,
			pageCount: 2,
			pageMover: &mockPageMover{
				firstSuccess:               true,
				secondSuccess:              true,
				firstStatus:                []int{0, 0},
				expectedPagesForSecondCall: 2,
			},
			targetNodes:                idset.NewIDSet(1, 2),
			expectedError:              false,
			expectedRemainingPageCount: 0,
		},
		{
			name: "move pages (only one)",
			pool: pagePool{
				pages: map[int][]page{
					500: {
						{
							pid:  500,
							addr: 0xdeadbeef,
						},
						{
							pid:  500,
							addr: 0xc0ffee,
						},
					},
				},
			},
			pid:       500,
			pageCount: 2,
			pageMover: &mockPageMover{
				firstSuccess:               true,
				secondSuccess:              true,
				firstStatus:                []int{0, 2},
				expectedPagesForSecondCall: 1,
			},
			targetNodes:                idset.NewIDSet(1, 2),
			expectedError:              false,
			expectedRemainingPageCount: 0,
		},
		{
			name: "move pages (none)",
			pool: pagePool{
				pages: map[int][]page{
					500: {
						{
							pid:  500,
							addr: 0xdeadbeef,
						},
						{
							pid:  500,
							addr: 0xc0ffee,
						},
					},
				},
			},
			pid:       500,
			pageCount: 2,
			pageMover: &mockPageMover{
				firstSuccess:               true,
				secondSuccess:              true,
				firstStatus:                []int{2, 1},
				expectedPagesForSecondCall: 0,
			},
			targetNodes:                idset.NewIDSet(1, 2),
			expectedError:              false,
			expectedRemainingPageCount: 0,
		},
		{
			name: "move pages (count 1)",
			pool: pagePool{
				pages: map[int][]page{
					500: {
						{
							pid:  500,
							addr: 0xdeadbeef,
						},
						{
							pid:  500,
							addr: 0xc0ffee,
						},
					},
				},
			},
			pid:       500,
			pageCount: 1,
			pageMover: &mockPageMover{
				firstSuccess:               true,
				secondSuccess:              true,
				firstStatus:                []int{0},
				expectedPagesForSecondCall: 1,
			},
			targetNodes:                idset.NewIDSet(1, 2),
			expectedError:              false,
			expectedRemainingPageCount: 1,
		},
		{
			name: "move pages (first call error)",
			pool: pagePool{
				pages: map[int][]page{
					500: {
						{
							pid:  500,
							addr: 0xdeadbeef,
						},
						{
							pid:  500,
							addr: 0xc0ffee,
						},
					},
				},
			},
			pid:       500,
			pageCount: 2,
			pageMover: &mockPageMover{
				firstSuccess:               false,
				secondSuccess:              true,
				firstStatus:                []int{0, 0},
				expectedPagesForSecondCall: 0,
			},
			targetNodes:                idset.NewIDSet(1, 2),
			expectedError:              true,
			expectedRemainingPageCount: 2,
		},
		{
			name: "move pages (second call error)",
			pool: pagePool{
				pages: map[int][]page{
					500: {
						{
							pid:  500,
							addr: 0xdeadbeef,
						},
						{
							pid:  500,
							addr: 0xc0ffee,
						},
					},
				},
			},
			pid:       500,
			pageCount: 2,
			pageMover: &mockPageMover{
				firstSuccess:               true,
				secondSuccess:              false,
				firstStatus:                []int{0, 0},
				expectedPagesForSecondCall: 0,
			},
			targetNodes:                idset.NewIDSet(1, 2),
			expectedError:              true,
			expectedRemainingPageCount: 2,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			dynamicDemoter := &demoter{
				maxPageMoveCount: tc.pageCount,
				pageMover:        tc.pageMover,
			}

			err := dynamicDemoter.movePages(tc.pool, tc.pageCount, tc.targetNodes)
			if err != nil {
				if err.Error() != "Fake error" {
					t.Errorf("Non-fake error: %v", err)
				}
			}
			if (err != nil) != tc.expectedError {
				t.Errorf("Unexpected error value")
			}

			if uint(len(tc.pool.pages[tc.pid])) != tc.expectedRemainingPageCount {
				t.Errorf("Wrong number of remaining pages: %d", len(tc.pool.pages[tc.pid]))
			}
		})
	}
}
