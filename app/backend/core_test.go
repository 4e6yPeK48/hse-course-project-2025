package main

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func apiRequest(t *testing.T, h http.Handler, method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var data bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &data)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
func responseMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestPublicIdentityCannotAuthorize(t *testing.T) {
	h := newServer().Handler()
	created := apiRequest(t, h, "POST", "/api/create", map[string]any{"team_names": []string{"Blue"}})
	data := responseMap(t, created)
	code := data["game_code"].(string)
	id := data["facilitator_id"].(string)
	if len(created.Result().Cookies()) != 1 {
		t.Fatal("creation must establish exactly one session cookie")
	}
	denied := apiRequest(t, h, "POST", "/api/game/"+code+"/start", map[string]any{"player_id": id})
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("public id authorized action: %d %s", denied.Code, denied.Body)
	}
	duplicate := apiRequest(t, h, "POST", "/api/join", map[string]any{"game_code": code, "nickname": "FACILITATOR", "team_id": "team-1"})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate recovered identity: %d", duplicate.Code)
	}
	me := apiRequest(t, h, "GET", "/api/game/"+code+"/me", nil, created.Result().Cookies()...)
	if me.Code != 200 || responseMap(t, me)["id"] != id {
		t.Fatalf("session lookup failed: %d %s", me.Code, me.Body)
	}
}

func openTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.db")
	s, err := OpenServer(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}
func createRoom(t *testing.T, h http.Handler) (string, string, []*http.Cookie) {
	t.Helper()
	r := apiRequest(t, h, "POST", "/api/create", map[string]any{"team_names": []string{"Blue"}})
	if r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	body := responseMap(t, r)
	return body["game_code"].(string), body["facilitator_id"].(string), r.Result().Cookies()
}
func joinRoom(t *testing.T, h http.Handler, code, nick string) (string, []*http.Cookie) {
	t.Helper()
	r := apiRequest(t, h, "POST", "/api/join", map[string]any{"game_code": code, "nickname": nick, "team_id": "team-1"})
	if r.Code != 201 {
		t.Fatalf("join: %d %s", r.Code, r.Body)
	}
	return responseMap(t, r)["player_id"].(string), r.Result().Cookies()
}
func firstProject(t *testing.T, h http.Handler, code string) string {
	t.Helper()
	body := responseMap(t, apiRequest(t, h, "GET", "/api/game/"+code, nil))
	return body["projects"].([]any)[0].(map[string]any)["id"].(string)
}
func mustAction(t *testing.T, h http.Handler, code, action string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := apiRequest(t, h, "POST", "/api/game/"+code+"/"+action, body, cookies...)
	if r.Code != 200 {
		t.Fatalf("%s: %d %s", action, r.Code, r.Body)
	}
	return r
}
func TestSessionsRejectSpoofingAndOtherRooms(t *testing.T) {
	s, _ := openTestServer(t)
	h := s.Handler()
	code, fac, cookies := createRoom(t, h)
	player, playerCookies := joinRoom(t, h, code, "Alice")
	_, _, otherCookies := createRoom(t, h)
	for name, c := range map[string][]*http.Cookie{"missing": nil, "other-room": otherCookies, "invalid": {{Name: cookieName(code), Value: fac}}} {
		t.Run(name, func(t *testing.T) {
			r := apiRequest(t, h, "POST", "/api/game/"+code+"/start_project", map[string]any{"player_id": fac, "project_id": "project-1"}, c...)
			if r.Code != 401 {
				t.Fatalf("status %d", r.Code)
			}
		})
	}
	r := apiRequest(t, h, "POST", "/api/game/"+code+"/start", map[string]any{"player_id": fac}, playerCookies...)
	if r.Code != 403 {
		t.Fatalf("spoof status %d", r.Code)
	}
	r = apiRequest(t, h, "GET", "/api/game/"+code+"/me", nil, playerCookies...)
	if responseMap(t, r)["id"] != player {
		t.Fatal("wrong player restored")
	}
	for _, tc := range []struct {
		cookies []*http.Cookie
		status  int
	}{{nil, 401}, {playerCookies, 403}, {cookies, 200}} {
		r = apiRequest(t, h, "GET", "/api/game/"+code+"/report", nil, tc.cookies...)
		if r.Code != tc.status {
			t.Fatalf("report: %d wanted %d", r.Code, tc.status)
		}
	}
	c := cookies[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 604800 || c.Secure {
		t.Fatalf("cookie attributes: %+v", c)
	}
	req := httptest.NewRequest("POST", "https://game.test/api/create", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !rec.Result().Cookies()[0].Secure {
		t.Fatal("HTTPS cookie missing Secure")
	}
	req = httptest.NewRequest("POST", "http://game.test/api/create", strings.NewReader(`{}`))
	req.Header.Set("Origin", "http://evil.test")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
}
func TestRestartPreservesCompleteGameAndSessions(t *testing.T) {
	s, path := openTestServer(t)
	h := s.Handler()
	code, _, fac := createRoom(t, h)
	player, pc := joinRoom(t, h, code, "Alice")
	project := firstProject(t, h, code)
	mustAction(t, h, code, "start_project", map[string]any{"project_id": project}, fac)
	mustAction(t, h, code, "start", map[string]any{}, fac)
	mustAction(t, h, code, "next_day", map[string]any{}, fac)
	mustAction(t, h, code, "skip_turn", map[string]any{}, fac)
	mustAction(t, h, code, "start_retro", map[string]any{}, fac)
	mustAction(t, h, code, "continue", map[string]any{}, fac)
	mustAction(t, h, code, "skip_turn", map[string]any{}, fac)
	before := apiRequest(t, h, "GET", "/api/game/"+code, nil).Body.String()
	privateBefore := s.snapshot()
	if !privateBefore.Games[code].TurnActionDone[player] || len(privateBefore.Games[code].DailySnapshots) == 0 || len(privateBefore.Games[code].Tasks) == 0 {
		t.Fatal("fixture did not exercise private state")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenServer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if !reflect.DeepEqual(privateBefore, restarted.snapshot()) {
		t.Fatal("private state changed over restart")
	}
	if got := apiRequest(t, restarted.Handler(), "GET", "/api/game/"+code, nil).Body.String(); got != before {
		t.Fatal("public state changed over restart")
	}
	for _, c := range [][]*http.Cookie{fac, pc} {
		if r := apiRequest(t, restarted.Handler(), "GET", "/api/game/"+code+"/me", nil, c...); r.Code != 200 {
			t.Fatal("session lost")
		}
	}
	// Starting another room uses persisted counters rather than reissuing an existing task ID.
	next, _, nextFac := createRoom(t, restarted.Handler())
	mustAction(t, restarted.Handler(), next, "start_project", map[string]any{"project_id": firstProject(t, restarted.Handler(), next)}, nextFac)
	for id := range restarted.games[next].Tasks {
		if privateBefore.Games[code].Tasks[id] != nil {
			t.Fatalf("task id reused: %s", id)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(fac[0].Value)) {
		t.Fatal("raw session secret persisted")
	}
}
func TestFailedSaveDoesNotPublishOrBroadcast(t *testing.T) {
	s, _ := openTestServer(t)
	h := s.Handler()
	code, _, cookies := createRoom(t, h)
	before := apiRequest(t, h, "GET", "/api/game/"+code, nil).Body.String()
	ts := httptest.NewServer(h)
	defer ts.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/ws/game?code="+code, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var initial gameSocketMessage
	if err := conn.ReadJSON(&initial); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	r := apiRequest(t, h, "POST", "/api/game/"+code+"/start_project", map[string]any{"project_id": firstProject(t, h, code)}, cookies...)
	if r.Code != 503 {
		t.Fatalf("failed storage acknowledged mutation: %d %s", r.Code, r.Body)
	}
	if got := apiRequest(t, h, "GET", "/api/game/"+code, nil).Body.String(); got != before {
		t.Fatal("failed save published state")
	}
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var update gameSocketMessage
	if err := conn.ReadJSON(&update); err == nil {
		t.Fatal("failed save broadcast state")
	}
}
func TestCommitRemainsInvisibleUntilSaved(t *testing.T) {
	s, _ := openTestServer(t)
	h := s.Handler()
	code, _, cookies := createRoom(t, h)
	previous := apiRequest(t, h, "GET", "/api/game/"+code, nil).Body.String()
	save := s.saveSnapshot
	entered := make(chan struct{})
	release := make(chan struct{})
	s.saveSnapshot = func(data []byte) error { close(entered); <-release; return save(data) }
	result := make(chan *httptest.ResponseRecorder, 1)
	project := firstProject(t, h, code)
	go func() {
		result <- apiRequest(t, h, "POST", "/api/game/"+code+"/start_project", map[string]any{"project_id": project}, cookies...)
	}()
	<-entered
	visible := apiRequest(t, h, "GET", "/api/game/"+code, nil).Body.String()
	close(release)
	r := <-result
	if visible != previous {
		t.Fatal("uncommitted candidate visible")
	}
	if r.Code != 200 {
		t.Fatalf("commit status %d", r.Code)
	}
	if responseMap(t, r)["revision"].(float64) != 2 {
		t.Fatalf("revision did not advance: %s", r.Body)
	}
}
func TestConcurrentCreationReadsAndMoves(t *testing.T) {
	s, _ := openTestServer(t)
	h := s.Handler()
	code, _, fac := createRoom(t, h)
	_, pc := joinRoom(t, h, code, "Alice")
	r := mustAction(t, h, code, "start_project", map[string]any{"project_id": firstProject(t, h, code)}, fac)
	tasks := responseMap(t, r)["teams"].([]any)[0].(map[string]any)["board"].(map[string]any)["ready"].([]any)
	task := tasks[0].(map[string]any)["id"].(string)
	mustAction(t, h, code, "start", map[string]any{}, fac)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); createRoom(t, h) }()
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				apiRequest(t, h, "GET", "/api/game/"+code, nil)
			}
		}()
		go func() {
			defer wg.Done()
			apiRequest(t, h, "POST", "/api/game/"+code+"/drag", map[string]any{"task_id": task, "to_stage": "in_progress"}, pc...)
		}()
	}
	wg.Wait()
}
func TestPublicStateDoesNotAliasTurnProgress(t *testing.T) {
	g := &Game{TurnActionDone: map[string]bool{"p": false}}
	state := stateFromGame(g)
	g.TurnActionDone["p"] = true
	if state.TurnActionDone["p"] {
		t.Fatal("public state aliases mutable turn progress")
	}
}

func TestSuccessfulBroadcastContainsCommittedRevision(t *testing.T) {
	s, path := openTestServer(t)
	h := s.Handler()
	code, _, fac := createRoom(t, h)
	ts := httptest.NewServer(h)
	defer ts.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/ws/game?code="+code, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var initial gameSocketMessage
	if err := conn.ReadJSON(&initial); err != nil {
		t.Fatal(err)
	}
	mustAction(t, h, code, "start_project", map[string]any{"project_id": firstProject(t, h, code)}, fac)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var update gameSocketMessage
	if err := conn.ReadJSON(&update); err != nil {
		t.Fatal(err)
	}
	if update.State.Revision != initial.State.Revision+1 {
		t.Fatalf("broadcast revision %d", update.State.Revision)
	}
	disk, err := OpenServer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	if disk.games[code].Revision != update.State.Revision {
		t.Fatal("broadcast precedes durable state")
	}
}
