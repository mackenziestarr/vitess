/*
Copyright 2019 The Vitess Authors.

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

package tabletserver

import (
	"html/template"
	"sort"
	"sync"
	"time"

	"context"
	"vitess.io/vitess/go/streamlog"
	"vitess.io/vitess/go/vt/callinfo"
	"vitess.io/vitess/go/vt/sqlparser"
)

// QueryDetail is a simple wrapper for Query, Context and a killable conn.
type QueryDetail struct {
	ctx    context.Context
	conn   killable
	connID int64
	start  time.Time
}

type killable interface {
	Current() string
	ID() int64
	Kill(message string, elapsed time.Duration) error
}

// NewQueryDetail creates a new QueryDetail
func NewQueryDetail(ctx context.Context, conn killable) *QueryDetail {
	return &QueryDetail{ctx: ctx, conn: conn, connID: conn.ID(), start: time.Now()}
}

// QueryList holds a thread safe list of QueryDetails
type QueryList struct {
	name string

	mu           sync.Mutex
	queryDetails map[int64]*QueryDetail
}

// NewQueryList creates a new QueryList
func NewQueryList(name string) *QueryList {
	return &QueryList{
		name:         name,
		queryDetails: make(map[int64]*QueryDetail),
	}
}

// Add adds a QueryDetail to QueryList
func (ql *QueryList) Add(qd *QueryDetail) {
	ql.mu.Lock()
	defer ql.mu.Unlock()
	ql.queryDetails[qd.connID] = qd
}

// Remove removes a QueryDetail from QueryList
func (ql *QueryList) Remove(qd *QueryDetail) {
	ql.mu.Lock()
	defer ql.mu.Unlock()
	delete(ql.queryDetails, qd.connID)
}

// Terminate updates the query status and kills the connection
func (ql *QueryList) Terminate(connID int64) bool {
	ql.mu.Lock()
	defer ql.mu.Unlock()
	qd := ql.queryDetails[connID]
	if qd == nil {
		return false
	}
	qd.conn.Kill("QueryList.Terminate()", time.Since(qd.start))
	return true
}

// TerminateAll terminates all queries and kills the MySQL connections
func (ql *QueryList) TerminateAll() {
	ql.mu.Lock()
	defer ql.mu.Unlock()
	for _, qd := range ql.queryDetails {
		qd.conn.Kill("QueryList.TerminateAll()", time.Since(qd.start))
	}
}

// QueryDetailzRow is used for rendering QueryDetail in a template
type QueryDetailzRow struct {
	Type              string
	Query             string
	ContextHTML       template.HTML
	Start             time.Time
	Duration          time.Duration
	ConnID            int64
	State             string
	ShowTerminateLink bool
}

type byStartTime []QueryDetailzRow

func (a byStartTime) Len() int           { return len(a) }
func (a byStartTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byStartTime) Less(i, j int) bool { return a[i].Start.Before(a[j].Start) }

// AppendQueryzRows returns a list of QueryDetailzRow sorted by start time
func (ql *QueryList) AppendQueryzRows(rows []QueryDetailzRow) []QueryDetailzRow {
	ql.mu.Lock()
	for _, qd := range ql.queryDetails {
		query := qd.conn.Current()
		if *streamlog.RedactDebugUIQueries {
			query, _ = sqlparser.RedactSQLQuery(query)
		}
		row := QueryDetailzRow{
			Type:        ql.name,
			Query:       query,
			ContextHTML: callinfo.HTMLFromContext(qd.ctx),
			Start:       qd.start,
			Duration:    time.Since(qd.start),
			ConnID:      qd.connID,
		}
		rows = append(rows, row)
	}
	ql.mu.Unlock()
	sort.Sort(byStartTime(rows))
	return rows
}
