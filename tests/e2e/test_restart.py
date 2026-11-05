from __future__ import annotations

import pytest

from tests.conftest import APIClient
from tests.utils.flows import create_game, join_player

pytestmark = pytest.mark.e2e


def test_room_sessions_and_private_turn_progress_survive_process_restart(restartable_backend):
    server = restartable_backend
    client = APIClient(server.base_url)
    code, facilitator_id = create_game(client, team_names=["One"])
    _, joined = join_player(client, code, "alice", "team-1")
    player_id = joined["player_id"]
    client.json("POST", f"/api/game/{code}/start_project", {"player_id": facilitator_id, "project_id": "PR-01"})
    _, state, _ = client.json("POST", f"/api/game/{code}/start", {"player_id": facilitator_id})
    task = state["teams"][0]["board"]["ready"][0]
    _, before, _ = client.json("POST", f"/api/game/{code}/drag", {"player_id": player_id, "task_id": task["id"], "to_stage": "in_progress"})
    _, report_before, _ = client.json("GET", f"/api/game/{code}/report", actor_id=facilitator_id)
    server.stop()
    server.start()
    _, restored, _ = client.json("GET", f"/api/game/{code}")
    assert restored == before
    _, me, _ = client.json("GET", f"/api/game/{code}/me", actor_id=player_id)
    assert me["id"] == player_id
    _, report_after, _ = client.json("GET", f"/api/game/{code}/report", actor_id=facilitator_id)
    assert report_after == report_before
    # Starting one task exhausted this player's turn. Restart must not grant a new one.
    status, _, _ = client.json("POST", f"/api/game/{code}/drag", {"player_id": player_id, "task_id": task["id"], "to_stage": "review"}, allow_error=True)
    assert status == 403
    _, next_day, _ = client.json("POST", f"/api/game/{code}/next_day", {"player_id": facilitator_id})
    assert next_day["revision"] > restored["revision"]
    assert next_day["current_day"] == 2
    new_code, new_facilitator = create_game(client, team_names=["Two"])
    _, new_join = join_player(client, new_code, "bob", "team-1")
    assert new_code != code
    assert {new_facilitator, new_join["player_id"]}.isdisjoint({facilitator_id, player_id})
