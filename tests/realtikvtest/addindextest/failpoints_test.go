// Copyright 2022 PingCAP, Inc.
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

package addindextest

import (
	"testing"

	"github.com/pingcap/tidb/tests/realtikvtest/addindextestutil"
)

func TestFailpointsCreateNonUniqueIndex(t *testing.T) {
	if !*FullMode {
		t.Skip()
	}
	var colIDs = [][]int{
		{1, 4, 7, 10, 13, 16, 19, 22, 25},
		{2, 5, 8, 11, 14, 17, 20, 23, 26},
		{3, 6, 9, 12, 15, 18, 21, 24, 27},
	}
	ctx := addindextestutil.InitTestFailpoint(t)
	addindextestutil.TestOneColFrame(ctx, colIDs, addindextestutil.AddIndexNonUnique)
}

func TestFailpointsCreateUniqueIndex(t *testing.T) {
	if !*FullMode {
		t.Skip()
	}
	var colIDs = [][]int{
		{1, 6, 7, 8, 11, 13, 15, 16, 18, 19, 22, 26},
		{2, 9, 11, 17},
		{3, 12, 25},
	}
	ctx := addindextestutil.InitTestFailpoint(t)
	addindextestutil.TestOneColFrame(ctx, colIDs, addindextestutil.AddIndexUnique)
}

func TestFailpointsCreatePrimaryKeyFailpoints(t *testing.T) {
	if !*FullMode {
		t.Skip()
	}
	ctx := addindextestutil.InitTest(t)
	addindextestutil.TestOneIndexFrame(ctx, 0, addindextestutil.AddIndexPK)
}

func TestFailpointsCreateGenColIndex(t *testing.T) {
	if !*FullMode {
		t.Skip()
	}
	ctx := addindextestutil.InitTestFailpoint(t)
	addindextestutil.TestOneIndexFrame(ctx, 29, addindextestutil.AddIndexGenCol)
}

func TestFailpointsCreateMultiColsIndex(t *testing.T) {
	if !*FullMode {
		t.Skip()
	}
	var coliIDs = [][]int{
		{1, 4, 7},
		{2, 5, 8},
		{3, 6, 9},
	}
	var coljIDs = [][]int{
		{16, 19, 22},
		{14, 17, 20},
		{18, 21, 24},
	}
	ctx := addindextestutil.InitTestFailpoint(t)
	addindextestutil.TestTwoColsFrame(ctx, coliIDs, coljIDs, addindextestutil.AddIndexMultiCols)
}
