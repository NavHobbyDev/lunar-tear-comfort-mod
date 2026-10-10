package userdata

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/store"
)

type DiffSet struct {
	updates map[string]string
	deletes map[string]string
}

func NewDiffSet(tables map[string]string) *DiffSet {
	return &DiffSet{
		updates: tables,
		deletes: make(map[string]string),
	}
}

func (ds *DiffSet) WithDeletes(table, deleteKeysJSON string) *DiffSet {
	if deleteKeysJSON != "" && deleteKeysJSON != "[]" {
		ds.deletes[table] = deleteKeysJSON
	}
	return ds
}

func (ds *DiffSet) Build() map[string]*pb.DiffData {
	diff := make(map[string]*pb.DiffData, len(ds.updates))
	for table, payload := range ds.updates {
		diff[table] = ds.entry(table, payload)
	}
	return diff
}

func (ds *DiffSet) BuildOrdered(order []string) map[string]*pb.DiffData {
	diff := make(map[string]*pb.DiffData, len(order))
	for _, table := range order {
		payload := ds.updates[table]
		diff[table] = ds.entry(table, payload)
	}
	return diff
}

func (ds *DiffSet) entry(table, payload string) *pb.DiffData {
	if payload == "" {
		payload = "[]"
	}
	deleteKeys := "[]"
	if dk, ok := ds.deletes[table]; ok {
		deleteKeys = dk
	}
	return &pb.DiffData{
		UpdateRecordsJson: payload,
		DeleteKeysJson:    deleteKeys,
	}
}

type trackedTable struct {
	tableName  string
	keyFields  []string
	oldRecords []map[string]any
	recordsFn  func(store.UserState) []map[string]any
}

type DeleteTracker struct {
	entries []trackedTable
}

func NewDeleteTracker() *DeleteTracker {
	return &DeleteTracker{}
}

func (dt *DeleteTracker) Track(tableName string, old store.UserState, recordsFn func(store.UserState) []map[string]any, keyFields []string) *DeleteTracker {
	dt.entries = append(dt.entries, trackedTable{
		tableName:  tableName,
		keyFields:  keyFields,
		oldRecords: recordsFn(old),
		recordsFn:  recordsFn,
	})
	return dt
}

func (dt *DeleteTracker) Apply(newState store.UserState, tables map[string]string) map[string]*pb.DiffData {
	ds := NewDiffSet(tables)
	for _, e := range dt.entries {
		newRecords := e.recordsFn(newState)
		ds.WithDeletes(e.tableName, ComputeDeleteKeys(e.oldRecords, newRecords, e.keyFields))
	}
	return ds.Build()
}

func ComputeDeleteKeys(oldRecords, newRecords []map[string]any, keyFields []string) string {
	if len(oldRecords) == 0 {
		return "[]"
	}

	newSet := make(map[string]struct{}, len(newRecords))
	for _, r := range newRecords {
		newSet[compositeKey(r, keyFields)] = struct{}{}
	}

	var deleted []map[string]any
	for _, r := range oldRecords {
		if _, exists := newSet[compositeKey(r, keyFields)]; !exists {
			deleted = append(deleted, r)
		}
	}

	if len(deleted) == 0 {
		return "[]"
	}
	b, err := json.Marshal(deleted)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func ComputeUpdateRecords(oldRecords, newRecords []map[string]any, keyFields []string) string {
	if len(newRecords) == 0 {
		return "[]"
	}

	oldByKey := make(map[string]map[string]any, len(oldRecords))
	for _, r := range oldRecords {
		oldByKey[compositeKey(r, keyFields)] = r
	}

	var changed []map[string]any
	for _, r := range newRecords {
		prev, exists := oldByKey[compositeKey(r, keyFields)]
		if !exists || !reflect.DeepEqual(prev, r) {
			changed = append(changed, r)
		}
	}

	if len(changed) == 0 {
		return "[]"
	}
	b, err := json.Marshal(changed)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func compositeKey(record map[string]any, fields []string) string {
	var sb strings.Builder
	for i, f := range fields {
		if i > 0 {
			sb.WriteByte('|')
		}
		sb.WriteString(fmt.Sprint(record[f]))
	}
	return sb.String()
}
