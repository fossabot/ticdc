// Copyright 2020 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package puller

import (
	"context"
	"math/rand"

	"github.com/pingcap/check"
	"github.com/pingcap/ticdc/cdc/model"
)

type mockEntrySorterSuite struct{}

var _ = check.Suite(&mockEntrySorterSuite{})

func (s *mockEntrySorterSuite) TestEntrySorter(c *check.C) {
	testCases := []struct {
		input      []*model.RawKVEntry
		resolvedTs uint64
		expect     []*model.RawKVEntry
	}{
		{
			input: []*model.RawKVEntry{
				{Ts: 1, OpType: model.OpTypePut},
				{Ts: 2, OpType: model.OpTypePut},
				{Ts: 4, OpType: model.OpTypeDelete},
				{Ts: 2, OpType: model.OpTypeDelete}},
			resolvedTs: 0,
			expect: []*model.RawKVEntry{
				{Ts: 0, OpType: model.OpTypeResolved}},
		},
		{
			input: []*model.RawKVEntry{
				{Ts: 3, OpType: model.OpTypePut},
				{Ts: 2, OpType: model.OpTypePut},
				{Ts: 5, OpType: model.OpTypePut}},
			resolvedTs: 3,
			expect: []*model.RawKVEntry{
				{Ts: 1, OpType: model.OpTypePut},
				{Ts: 2, OpType: model.OpTypeDelete},
				{Ts: 2, OpType: model.OpTypePut},
				{Ts: 2, OpType: model.OpTypePut},
				{Ts: 3, OpType: model.OpTypePut},
				{Ts: 3, OpType: model.OpTypeResolved}},
		},
		{
			input: []*model.RawKVEntry{
				{Ts: 7, OpType: model.OpTypePut}},
			resolvedTs: 6,
			expect: []*model.RawKVEntry{
				{Ts: 4, OpType: model.OpTypeDelete},
				{Ts: 5, OpType: model.OpTypePut},
				{Ts: 6, OpType: model.OpTypeResolved}},
		},
		{
			input:      []*model.RawKVEntry{{Ts: 7, OpType: model.OpTypeDelete}},
			resolvedTs: 6,
			expect: []*model.RawKVEntry{
				{Ts: 6, OpType: model.OpTypeResolved}},
		},
		{
			input:      []*model.RawKVEntry{{Ts: 7, OpType: model.OpTypeDelete}},
			resolvedTs: 8,
			expect: []*model.RawKVEntry{
				{Ts: 7, OpType: model.OpTypeDelete},
				{Ts: 7, OpType: model.OpTypeDelete},
				{Ts: 7, OpType: model.OpTypePut},
				{Ts: 8, OpType: model.OpTypeResolved}},
		},
		{
			input:      []*model.RawKVEntry{},
			resolvedTs: 15,
			expect: []*model.RawKVEntry{
				{Ts: 15, OpType: model.OpTypeResolved}},
		},
	}
	es := NewEntrySorter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Run(ctx)
	for _, tc := range testCases {
		for _, entry := range tc.input {
			es.AddEntry(entry)
		}
		es.AddEntry(&model.RawKVEntry{Ts: tc.resolvedTs, OpType: model.OpTypeResolved})
		for i := 0; i < len(tc.expect); i++ {
			e := <-es.Output()
			c.Check(e, check.DeepEquals, tc.expect[i])
		}
	}
}

func (s *mockEntrySorterSuite) TestEntrySorterRandomly(c *check.C) {
	es := NewEntrySorter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Run(ctx)

	maxTs := uint64(100000)
	go func() {
		for resolvedTs := uint64(1); resolvedTs <= maxTs; resolvedTs += 400 {
			var opType model.OpType
			if rand.Intn(2) == 0 {
				opType = model.OpTypePut
			} else {
				opType = model.OpTypeDelete
			}
			for i := 0; i < 1000; i++ {
				entry := &model.RawKVEntry{
					Ts:     uint64(int64(resolvedTs) + rand.Int63n(int64(maxTs-resolvedTs))),
					OpType: opType,
				}
				es.AddEntry(entry)
			}
			es.AddEntry(&model.RawKVEntry{Ts: resolvedTs, OpType: model.OpTypeResolved})
		}
		es.AddEntry(&model.RawKVEntry{Ts: maxTs, OpType: model.OpTypeResolved})
	}()
	var lastTs uint64
	var resolvedTs uint64
	lastOpType := model.OpTypePut
	for entry := range es.Output() {
		c.Assert(entry.Ts, check.GreaterEqual, lastTs)
		c.Assert(entry.Ts, check.Greater, resolvedTs)
		if lastOpType == model.OpTypePut && entry.OpType == model.OpTypeDelete {
			c.Assert(entry.Ts, check.Greater, lastTs)
		}
		lastTs = entry.Ts
		lastOpType = entry.OpType
		if entry.OpType == model.OpTypeResolved {
			resolvedTs = entry.Ts
		}
		if resolvedTs == maxTs {
			break
		}
	}
}
