from __future__ import annotations

import pytest


def create_game(api_client, *, team_names=None, max_days=10):
    payload = {
        "team_names": team_names or ["Blue", "Green"],
        "max_days": max_days,
    }
    status, data, _ = api_client.json("POST", "/api/create", payload)
    assert status == 201
    return data["game_code"], data["facilitator_id"]


def join_player(api_client, code: str, nickname: str, team_id: str):
    status, data, _ = api_client.json(
        "POST",
        "/api/join",
        {"game_code": code, "nickname": nickname, "team_id": team_id},
    )
    return status, data


def start_running_game(api_client, *, team_names=None, max_days=10):
    code, facilitator_id = create_game(
        api_client,
        team_names=team_names or ["Solo Team"],
        max_days=max_days,
    )
    for idx, _ in enumerate(team_names or ["Solo Team"], start=1):
        status, _ = join_player(api_client, code, f"player-{idx}", f"team-{idx}")
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
    return code, facilitator_id, state


def advance_to_retro(api_client, code: str, facilitator_id: str):
    _, state, _ = api_client.json("GET", f"/api/game/{code}")
    while not state.get("next_day_is_retro", False):
        status, state, _ = api_client.json(
            "POST", f"/api/game/{code}/next_day", {"player_id": facilitator_id},
        )
        assert status == 200
    status, rejected, _ = api_client.json(
        "POST", f"/api/game/{code}/next_day", {"player_id": facilitator_id}, allow_error=True,
    )
    assert status == 409
    assert "ретро" in rejected["error"]
    status, state, _ = api_client.json(
        "POST", f"/api/game/{code}/start_retro", {"player_id": facilitator_id},
    )
    assert status == 200
    assert state["phase"] == "retro"
    return state


def finish_game(api_client, code: str, facilitator_id: str, *, max_days=1500):
    """Play only public HTTP actions, choosing legal moves from the visible board."""
    _, state, _ = api_client.json("GET", f"/api/game/{code}")
    for _ in range(max_days):
        if state["finished"]:
            assert all(project["completed"] for project in state["projects"])
            return state
        if state["phase"] == "retro":
            _, state, _ = api_client.json(
                "POST", f"/api/game/{code}/continue", {"player_id": facilitator_id},
            )
        for project in state["projects"]:
            if not project["started"]:
                status, result, _ = api_client.json(
                    "POST", f"/api/game/{code}/start_project",
                    {"player_id": facilitator_id, "project_id": project["id"]}, allow_error=True,
                )
                if status != 200:
                    assert status == 409, result
                    break
                state = result
        players = [p["id"] for t in state["teams"] for p in t["members"]]
        for player_id in players:
            for _ in range(2):  # Heads can require a block followed by starting/helping.
                team = next(t for t in state["teams"] if any(p["id"] == player_id for p in t["members"]))
                player = next(p for p in team["members"] if p["id"] == player_id)
                candidates = []
                for stage in ("review", "in_progress", "ready"):
                    for task in team["board"][stage]:
                        if task["blocked"] or (player["current_coin"] == "heads" and stage != "ready"):
                            candidates.append((task["id"], stage))
                        if not task["blocked"]:
                            candidates.append((task["id"], {"ready": "in_progress", "in_progress": "review", "review": "done"}[stage]))
                for task_id, to_stage in candidates:
                    status, result, _ = api_client.json(
                        "POST", f"/api/game/{code}/drag",
                        {"player_id": player_id, "task_id": task_id, "to_stage": to_stage}, allow_error=True,
                    )
                    if status == 200:
                        state = result
                        break
                    assert status in (403, 409), result
                else:
                    break
                if state["finished"]:
                    assert all(project["completed"] for project in state["projects"])
                    return state
        action = "start_retro" if state.get("next_day_is_retro", False) else "next_day"
        _, state, _ = api_client.json(
            "POST", f"/api/game/{code}/{action}", {"player_id": facilitator_id},
        )
    pytest.fail(f"Game did not finish within {max_days} days: day={state['current_day']} projects_done={state['projects_done']}")


def create_game_with_coin_outcome(api_client, desired_coin: str, *, max_attempts=30):
    for _ in range(max_attempts):
        code, facilitator_id, state = start_running_game(api_client)
        player = state["teams"][0]["members"][0]
        if player.get("current_coin") == desired_coin:
            return code, facilitator_id, state

    pytest.fail(f"Could not get coin={desired_coin} after {max_attempts} attempts")


def get_player_with_coin(api_client, code: str, desired_coin: str):
    status, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert status == 200

    for team in state["teams"]:
        for player in team["members"]:
            if player.get("current_coin") == desired_coin:
                return player["id"]

    return None
