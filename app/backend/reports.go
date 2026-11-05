package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
)

type DaySnapshot struct {
	Day          int         `json:"day"`
	Iteration    int         `json:"iteration"`
	Partial      bool        `json:"partial"`
	Metrics      GameMetrics `json:"metrics"`
	EndIteration bool        `json:"-"`
}

type IterationReport struct {
	Number     int     `json:"number"`
	StartDay   int     `json:"start_day"`
	EndDay     int     `json:"end_day"`
	Partial    bool    `json:"partial"`
	Throughput int     `json:"throughput"`
	AverageWIP float64 `json:"average_wip"`
	LeadTime   float64 `json:"lead_time"`
	CycleTime  float64 `json:"cycle_time"`
}

type GameReport struct {
	Code       string            `json:"code"`
	Revision   uint64            `json:"revision"`
	Phase      string            `json:"phase"`
	Finished   bool              `json:"finished"`
	Summary    GameMetrics       `json:"summary"`
	Days       []DaySnapshot     `json:"days"`
	Iterations []IterationReport `json:"iterations"`
}

func captureDay(g *Game, endIteration, partial bool) {
	snapshot := DaySnapshot{Day: g.CurrentDay, Iteration: g.CyclesCompleted + 1, Partial: partial, Metrics: *calculateMetrics(g), EndIteration: endIteration}
	if n := len(g.DailySnapshots); n > 0 {
		last := g.DailySnapshots[n-1]
		snapshot.Iteration = last.Iteration
		if last.Day == snapshot.Day {
			snapshot.EndIteration = last.EndIteration || endIteration
			g.DailySnapshots[n-1] = snapshot
			return
		}
		if last.EndIteration {
			snapshot.Iteration++
		}
	}
	g.DailySnapshots = append(g.DailySnapshots, snapshot)
}

func buildReport(g *Game) GameReport {
	report := GameReport{
		Code: g.Code, Revision: g.Revision, Phase: g.Phase, Finished: g.Finished,
		Summary: *calculateMetrics(g), Days: make([]DaySnapshot, 0, len(g.DailySnapshots)+1), Iterations: []IterationReport{},
	}
	for _, snapshot := range g.DailySnapshots {
		metrics := snapshot.Metrics
		metrics.CFD = make(map[string]int, len(snapshot.Metrics.CFD))
		for stage, count := range snapshot.Metrics.CFD {
			metrics.CFD[stage] = count
		}
		snapshot.Metrics = metrics
		report.Days = append(report.Days, snapshot)
	}
	if g.Started && !g.Finished && g.Phase == "running" {
		live := DaySnapshot{Day: g.CurrentDay, Iteration: g.CyclesCompleted + 1, Partial: true, Metrics: report.Summary}
		if n := len(report.Days); n > 0 {
			last := report.Days[n-1]
			live.Iteration = last.Iteration
			if last.EndIteration {
				live.Iteration++
			}
			if last.Day == live.Day {
				live.Iteration, live.EndIteration = last.Iteration, last.EndIteration
				report.Days[n-1] = live
			} else {
				report.Days = append(report.Days, live)
			}
		} else {
			report.Days = append(report.Days, live)
		}
	}

	previousDone := 0
	for start := 0; start < len(report.Days); {
		end := start
		wip := 0
		for end < len(report.Days) && report.Days[end].Iteration == report.Days[start].Iteration {
			wip += report.Days[end].Metrics.WIP
			end++
		}
		first, last := report.Days[start], report.Days[end-1]
		iteration := IterationReport{
			Number: first.Iteration, StartDay: first.Day, EndDay: last.Day, Partial: !last.EndIteration,
			Throughput: last.Metrics.CFD["done"] - previousDone, AverageWIP: float64(wip) / float64(end-start),
		}
		leadDays, projects := 0, 0
		for _, project := range g.Projects {
			if project.Completed && project.DoneDay >= first.Day && project.DoneDay <= last.Day {
				leadDays += project.DoneDay - project.StartedDay + 1
				projects++
			}
		}
		if projects > 0 {
			iteration.LeadTime = float64(leadDays) / float64(projects)
		}
		cycleDays, tasks := 0, 0
		for _, task := range g.Tasks {
			if task.Stage == "done" && task.StartedDay > 0 && task.DoneDay >= task.StartedDay && task.DoneDay >= first.Day && task.DoneDay <= last.Day {
				cycleDays += task.DoneDay - task.StartedDay + 1
				tasks++
			}
		}
		if tasks > 0 {
			iteration.CycleTime = float64(cycleDays) / float64(tasks)
		}
		report.Iterations = append(report.Iterations, iteration)
		previousDone = last.Metrics.CFD["done"]
		start = end
	}
	return report
}

func writeGameReport(w http.ResponseWriter, r *http.Request, g *Game) {
	report := buildReport(g)
	if r.URL.Query().Get("format") != "csv" {
		writeJSON(w, http.StatusOK, report)
		return
	}
	var body bytes.Buffer
	writer := csv.NewWriter(&body)
	_ = writer.Write([]string{"code", "revision", "day", "iteration", "partial", "ready", "in_progress", "review", "completed_tasks", "throughput", "wip", "lead_time", "cycle_time", "blocked", "total_blocked_days", "total_penalties", "retro_days", "velocity", "last_retro_throughput"})
	previousDone := 0
	decimal := func(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }
	for _, day := range report.Days {
		m := day.Metrics
		_ = writer.Write([]string{
			report.Code, strconv.FormatUint(report.Revision, 10), strconv.Itoa(day.Day), strconv.Itoa(day.Iteration), strconv.FormatBool(day.Partial),
			strconv.Itoa(m.CFD["ready"]), strconv.Itoa(m.CFD["in_progress"]), strconv.Itoa(m.CFD["review"]), strconv.Itoa(m.CFD["done"]),
			strconv.Itoa(m.CFD["done"] - previousDone), strconv.Itoa(m.WIP), decimal(m.LeadTime), decimal(m.AvgTaskCycleTime),
			strconv.Itoa(m.Blocked), strconv.Itoa(m.TotalBlockedDays), strconv.Itoa(m.TotalPenalties), strconv.Itoa(m.RetroDays), decimal(m.Velocity), strconv.Itoa(m.LastRetroThroughput),
		})
		previousDone = m.CFD["done"]
	}
	writer.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="featureban-%s-report.csv"`, report.Code))
	_, _ = w.Write(body.Bytes())
}
