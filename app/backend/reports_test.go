package main

import (
	"encoding/csv"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func reportFixture() *Game {
	return &Game{
		Code: "TEST", Revision: 7, Started: true, Phase: "running", CurrentDay: 1,
		Teams: map[string]*Team{"team": {Board: map[string][]string{"in_progress": {"a", "b"}}}},
		Tasks: map[string]*Task{
			"a": {Stage: "in_progress", StartedDay: 1},
			"b": {Stage: "in_progress", StartedDay: 1},
		},
		Projects: map[string]*ProjectCard{"first": {StartedDay: 1}, "second": {StartedDay: 2}},
	}
}

func TestReportRetainsDailyHistoryAndRealIterationBoundaries(t *testing.T) {
	g := reportFixture()
	captureDay(g, false, false)
	g.CurrentDay = 2
	g.Teams["team"].Board = map[string][]string{"in_progress": {"b"}, "done": {"a"}}
	g.Tasks["a"].Stage, g.Tasks["a"].DoneDay = "done", 2
	g.Projects["first"].Completed, g.Projects["first"].DoneDay = true, 2
	captureDay(g, true, false)
	g.CurrentDay, g.CyclesCompleted, g.Phase = 3, 1, "retro"

	report := buildReport(g)
	want := []IterationReport{{Number: 1, StartDay: 1, EndDay: 2, Throughput: 1, AverageWIP: 1.5, LeadTime: 2, CycleTime: 2}}
	if !reflect.DeepEqual(report.Iterations, want) || len(report.Days) != 2 {
		t.Fatalf("retro report = %+v, days = %+v", report.Iterations, report.Days)
	}
	if report.Code != "TEST" || report.Revision != 7 || report.Finished || report.Phase != "retro" {
		t.Fatalf("report identity = %+v", report)
	}
	if report.Days[0].Metrics.CFD["done"] != 0 || report.Days[0].Metrics.WIP != 2 {
		t.Fatalf("old day changed with board: %+v", report.Days[0])
	}

	g.Phase = "running"
	live := buildReport(g)
	if len(live.Days) != 3 || !live.Days[2].Partial || !live.Iterations[1].Partial || len(g.DailySnapshots) != 2 {
		t.Fatalf("live report must include one unsaved partial day: %+v", live)
	}
	g.Teams["team"].Board = map[string][]string{"done": {"a", "b"}}
	g.Tasks["b"].Stage, g.Tasks["b"].DoneDay = "done", 3
	g.Projects["second"].Completed, g.Projects["second"].DoneDay = true, 3
	captureDay(g, false, true)
	captureDay(g, false, true)
	g.Finished, g.Phase = true, "finished"
	report = buildReport(g)
	want = append(want, IterationReport{Number: 2, StartDay: 3, EndDay: 3, Partial: true, Throughput: 1, LeadTime: 2, CycleTime: 3})
	if !reflect.DeepEqual(report.Iterations, want) || len(report.Days) != 3 || !report.Days[2].Partial {
		t.Fatalf("finished report = %+v, days = %+v", report.Iterations, report.Days)
	}
	if report.Summary.CFD["done"] != 2 || report.Summary.AvgTaskCycleTime != 2.5 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	report.Days[0].Metrics.CFD["done"] = 99
	if g.DailySnapshots[0].Metrics.CFD["done"] != 0 {
		t.Fatal("report exposes the stored snapshot map")
	}
}

func TestReportCSVUsesSameCumulativeDoneAndDailyThroughput(t *testing.T) {
	g := reportFixture()
	g.Teams["team"].Board = map[string][]string{"done": {"a"}, "in_progress": {"b"}}
	captureDay(g, false, false)
	g.CurrentDay = 2
	g.Teams["team"].Board = map[string][]string{"done": {"a", "b"}}
	response := httptest.NewRecorder()
	writeGameReport(response, httptest.NewRequest("GET", "/report?format=csv", nil), g)
	rows, err := csv.NewReader(response.Body).ReadAll()
	if err != nil || len(rows) != 3 {
		t.Fatalf("CSV rows = %v, err = %v", rows, err)
	}
	columns := map[string]int{}
	for i, column := range rows[0] {
		columns[column] = i
	}
	for column, want := range map[string][]string{
		"day": {"1", "2"}, "iteration": {"1", "1"}, "partial": {"false", "true"},
		"completed_tasks": {"1", "2"}, "throughput": {"1", "1"}, "wip": {"1", "0"},
	} {
		i, ok := columns[column]
		if !ok || rows[1][i] != want[0] || rows[2][i] != want[1] {
			t.Errorf("CSV column %s: rows=%v", column, rows)
		}
	}
	if !strings.Contains(response.Header().Get("Content-Disposition"), "TEST") {
		t.Fatalf("CSV download header = %s", response.Header().Get("Content-Disposition"))
	}
	response = httptest.NewRecorder()
	writeGameReport(response, httptest.NewRequest("GET", "/report", nil), g)
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 7 || len(g.DailySnapshots) != 1 {
		t.Fatalf("JSON contract or read-only report broken: %s", response.Body)
	}
	if strings.Contains(response.Body.String(), "EndIteration") || strings.Contains(response.Body.String(), "end_iteration") {
		t.Fatalf("internal boundary leaked in JSON: %s", response.Body)
	}
}

func TestReportLobbyHasNoInventedDayOrIteration(t *testing.T) {
	report := buildReport(&Game{Code: "LOBBY", CurrentDay: 1, Phase: "lobby"})
	if report.Days == nil || len(report.Days) != 0 || report.Iterations == nil || len(report.Iterations) != 0 {
		t.Fatalf("lobby report = %+v", report)
	}
}
