package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type session struct {
	PlayerID  string
	ExpiresAt time.Time
}
type serverSnapshot struct {
	Games       map[string]*Game
	GameCounter int64
	TaskCounter int64
	Sessions    map[string]map[string]session
}

func encodeSnapshot(snapshot serverSnapshot) ([]byte, error) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(snapshot)
	return b.Bytes(), err
}
func decodeSnapshot(data []byte) (serverSnapshot, error) {
	var snapshot serverSnapshot
	err := gob.NewDecoder(bytes.NewReader(data)).Decode(&snapshot)
	return snapshot, err
}
func (s *Server) snapshot() serverSnapshot {
	return serverSnapshot{s.games, s.gameCounter, s.taskCounter, s.sessions}
}
func (s *Server) restore(snapshot serverSnapshot) {
	s.games, s.gameCounter, s.taskCounter, s.sessions = snapshot.Games, snapshot.GameCounter, snapshot.TaskCounter, snapshot.Sessions
}

// OpenServer loads the complete committed state for this single-process server.
func OpenServer(path string) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Server, error) { db.Close(); return nil, err }
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; CREATE TABLE IF NOT EXISTS server_snapshot (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL)`); err != nil {
		return fail(err)
	}
	s := newServer()
	s.db = db
	var data []byte
	err = db.QueryRow(`SELECT payload FROM server_snapshot WHERE id=1`).Scan(&data)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fail(err)
	}
	if err == nil {
		snapshot, err := decodeSnapshot(data)
		if err != nil {
			return fail(fmt.Errorf("decode persisted games: %w", err))
		}
		s.restore(snapshot)
	}
	s.saveSnapshot = func(payload []byte) error {
		_, err := db.Exec(`INSERT INTO server_snapshot(id,payload) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`, payload)
		return err
	}
	return s, nil
}
func (s *Server) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return err == nil && u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
}
func cookieName(code string) string { return "featureban_" + code }
func sessionHash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
func (s *Server) establishSession(w http.ResponseWriter, r *http.Request, code, playerID string) error {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	token := hex.EncodeToString(secret)
	expires := time.Now().Add(7 * 24 * time.Hour)
	if s.sessions[code] == nil {
		s.sessions[code] = make(map[string]session)
	}
	s.sessions[code][sessionHash(token)] = session{playerID, expires}
	http.SetCookie(w, &http.Cookie{Name: cookieName(code), Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 60 * 60, Expires: expires})
	return nil
}
func (s *Server) actor(r *http.Request, code string) (*Player, bool) {
	c, err := r.Cookie(cookieName(code))
	if err != nil {
		return nil, false
	}
	sess, ok := s.sessions[code][sessionHash(c.Value)]
	if !ok || !sess.ExpiresAt.After(time.Now()) {
		return nil, false
	}
	g := s.games[code]
	if g == nil {
		return nil, false
	}
	p, ok := g.Players[sess.PlayerID]
	return p, ok
}

// Handler is the production router, including authentication and commit-before-publish.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/hello", s.handleHello)
	mux.HandleFunc("/api/", s.handleJoinRedirect)
	mux.HandleFunc("/api/create", s.handleCreateGame)
	mux.HandleFunc("/api/join", s.handleJoinGame)
	mux.HandleFunc("/api/game/", s.gameRoute)
	mux.HandleFunc("/ws/lobby", s.handleLobbyWS)
	mux.HandleFunc("/ws/game", s.handleGameWS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutation := r.Method == http.MethodPost && (r.URL.Path == "/api/create" || r.URL.Path == "/api/join" || strings.HasPrefix(r.URL.Path, "/api/game/"))
		if !mutation {
			mux.ServeHTTP(w, r)
			return
		}
		if !sameOrigin(r) {
			errorJSON(w, http.StatusForbidden, "Запрос с другого сайта запрещён")
			return
		}
		s.mutationMu.Lock()
		defer s.mutationMu.Unlock()
		s.mu.RLock()
		data, err := encodeSnapshot(s.snapshot())
		previousRevisions := make(map[string]uint64, len(s.games))
		for code, g := range s.games {
			previousRevisions[code] = g.Revision
		}
		s.mu.RUnlock()
		if err != nil {
			errorJSON(w, 500, "Не удалось подготовить изменение")
			return
		}
		snapshot, err := decodeSnapshot(data)
		if err != nil {
			errorJSON(w, 500, "Не удалось подготовить изменение")
			return
		}
		candidate := newServer()
		candidate.restore(snapshot)
		buffered := &bufferedResponse{header: make(http.Header)}
		candidate.rawMutation(buffered, r)
		if buffered.status >= 200 && buffered.status < 400 {
			data, err = encodeSnapshot(candidate.snapshot())
			if err == nil && s.saveSnapshot != nil {
				err = s.saveSnapshot(data)
			}
			if err != nil {
				errorJSON(w, http.StatusServiceUnavailable, "Не удалось сохранить игру. Повторите действие.")
				return
			}
			s.mu.Lock()
			s.restore(candidate.snapshot())
			s.mu.Unlock()
			for _, flush := range candidate.metricWrites {
				flush()
			}
			for code, g := range candidate.games {
				if previous := previousRevisions[code]; g.Revision != previous {
					s.broadcastGameState(code, stateFromGame(g))
				}
			}
		}
		for k, values := range buffered.header {
			w.Header()[k] = values
		}
		w.WriteHeader(buffered.status)
		_, _ = w.Write(buffered.body.Bytes())
	})
}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *bufferedResponse) Header() http.Header { return b.header }
func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *bufferedResponse) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	return b.body.Write(data)
}
func (s *Server) rawMutation(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/create":
		s.handleCreateGame(w, r)
	case "/api/join":
		s.handleJoinGame(w, r)
	default:
		s.gameRoute(w, r)
	}
}
func (s *Server) gameRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/game/"), "/")
	code := strings.TrimSpace(parts[0])
	privateRead := len(parts) == 2 && (parts[1] == "me" || parts[1] == "report") && r.Method == http.MethodGet
	if r.Method == http.MethodPost || privateRead {
		w.Header().Set("Cache-Control", "no-store")
		s.mu.RLock()
		actor, ok := s.actor(r, code)
		if !ok {
			s.mu.RUnlock()
			errorJSON(w, http.StatusUnauthorized, "Войдите в игру заново")
			return
		}
		player := *actor
		if privateRead {
			if parts[1] == "me" {
				s.mu.RUnlock()
				writeJSON(w, 200, player)
				return
			}
			if player.Role != "facilitator" {
				s.mu.RUnlock()
				errorJSON(w, 403, "Отчёт доступен только ведущему")
				return
			}
			writeGameReport(w, r, s.games[code])
			s.mu.RUnlock()
			return
		}
		s.mu.RUnlock()
		// Legacy handlers receive only the authenticated actor, never caller-provided authority.
		var body map[string]json.RawMessage
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil && err != io.EOF {
			errorJSON(w, 400, "Некорректный запрос")
			return
		}
		if raw, exists := body["player_id"]; exists {
			var id string
			if json.Unmarshal(raw, &id) != nil || id != "" && id != player.ID {
				errorJSON(w, 403, "Нельзя действовать от имени другого игрока")
				return
			}
		}
		if body == nil {
			body = make(map[string]json.RawMessage)
		}
		body["player_id"], _ = json.Marshal(player.ID)
		encoded, _ := json.Marshal(body)
		r = r.Clone(r.Context())
		r.Body = io.NopCloser(bytes.NewReader(encoded))
		r.Header.Set("Content-Type", "application/json")
		s.games[code].Revision++
	}
	s.handleGameRoutes(w, r)
}
func cloneBoolMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
