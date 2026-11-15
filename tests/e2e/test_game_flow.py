from __future__ import annotations

import pytest

from tests.utils.flows import advance_to_retro, create_game, finish_game, join_player


pytestmark = pytest.mark.e2e


def test_full_game_cycle_to_retro_and_continue(api_client):
    code, facilitator_id = create_game(api_client, team_names=["QA Team"], max_days=10)
    status, data = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    player_id = data["player_id"]
    assert player_id

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    assert status == 200
    assert state["projects"][0]["started"] is True

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )
    assert status == 200
    assert state["phase"] == "running"
    assert state["started"] is True

    state = advance_to_retro(api_client, code, facilitator_id)
    assert state["current_day"] == 5

    team = state["teams"][0]
    previous_wip = team["wip_limits"]["in_progress"]
    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/set_wip",
        {"player_id": facilitator_id, "team_id": team["id"], "stage": "in_progress", "wip_limit": previous_wip + 1},
    )
    assert status == 200
    updated_team = next(item for item in state["teams"] if item["id"] == team["id"])
    assert updated_team["wip_limits"]["in_progress"] == previous_wip + 1

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/continue",
        {"player_id": facilitator_id},
    )
    assert status == 200
    assert state["phase"] == "running"
    assert state["current_day"] == 5
    assert state["teams"][0]["wip_limits"]["in_progress"] == previous_wip + 1


def test_permissions_and_phase_guards_during_running_game(api_client):
    code, facilitator_id = create_game(api_client, team_names=["QA", "Dev"], max_days=10)
    status, data = join_player(api_client, code, "alice", "team-1")
    assert status == 201
    player_a = data["player_id"]
    status, _ = join_player(api_client, code, "bob", "team-2")
    assert status == 201

    status, _, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    assert status == 200

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )
    assert status == 200
    assert state["phase"] == "running"

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/next_day",
        {"player_id": player_a},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "это может сделать только ведущий"

    team1 = next(team for team in state["teams"] if team["id"] == "team-1")
    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/set_wip",
        {"player_id": facilitator_id, "team_id": team1["id"], "stage": "in_progress", "wip_limit": team1["wip_limit"] + 1},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "Лимит WIP можно менять только на ретро или до начала игры"

    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert status == 200
    team2 = next(team for team in state["teams"] if team["id"] == "team-2")
    foreign_task = team2["board"]["ready"][0]["id"]

    status, data, _ = api_client.json(
        "POST",
        f"/api/game/{code}/drag",
        {"player_id": player_a, "task_id": foreign_task, "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 403
    assert data["error"] == "Задача принадлежит другой команде"

    for _ in range(2):
        status, _, _ = api_client.json(
            "POST",
            f"/api/game/{code}/skip_turn",
            {"player_id": facilitator_id},
        )
        assert status == 200

    status, state, _ = api_client.json(
        "POST",
        f"/api/game/{code}/next_day",
        {"player_id": facilitator_id},
    )
    assert status == 200
    assert state["current_day"] == 2


def test_max_days_does_not_finish_game_with_incomplete_projects(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"], max_days=5)
    status, _ = join_player(api_client, code, "alice", "team-1")
    assert status == 201

    api_client.json(
        "POST",
        f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST",
        f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )

    state = advance_to_retro(api_client, code, facilitator_id)
    assert state["current_day"] == 5
    assert state["finished"] is False
    _, state, _ = api_client.json("POST", f"/api/game/{code}/continue", {"player_id": facilitator_id})
    _, state, _ = api_client.json("POST", f"/api/game/{code}/next_day", {"player_id": facilitator_id})
    assert state["current_day"] == 6
    assert state["finished"] is False
    assert state["phase"] == "running"


def test_all_projects_finish_and_report_exports_final_partial_interval(api_client):
    code, facilitator_id = create_game(api_client, team_names=["Team A"], max_days=5)
    _, joined = join_player(api_client, code, "alice", "team-1")
    player_id = joined["player_id"]

    api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
    )
    api_client.json(
        "POST", f"/api/game/{code}/start",
        {"player_id": facilitator_id},
    )

    state = finish_game(api_client, code, facilitator_id)
    assert state["finished"] is True
    assert state["phase"] == "finished"
    assert state["projects_done"] == 15
    assert all(project["completed"] for project in state["projects"])
    _, report, _ = api_client.json("GET", f"/api/game/{code}/report", actor_id=facilitator_id)
    assert report["finished"] is True
    assert report["revision"] == state["revision"]
    assert report["days"][-1]["day"] == state["current_day"]
    assert report["days"][-1]["partial"] is True
    assert report["iterations"][-1]["partial"] is True
    assert len({day["day"] for day in report["days"]}) == len(report["days"])
    assert sum(period["throughput"] for period in report["iterations"]) == state["metrics"]["cfd"]["done"]
    import csv
    import io
    _, exported, _ = api_client.request("GET", f"/api/game/{code}/report?format=csv", actor_id=facilitator_id)
    rows = list(csv.DictReader(io.StringIO(exported)))
    assert len(rows) == len(report["days"])
    assert int(rows[-1]["completed_tasks"]) == state["metrics"]["cfd"]["done"]

    status, data, _ = api_client.json(
        "POST", f"/api/game/{code}/drag",
        payload={"player_id": player_id, "task_id": "some-task", "to_stage": "in_progress"},
        allow_error=True,
    )
    assert status == 409
    assert data["error"] == "Игра уже завершена"
