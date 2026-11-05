from __future__ import annotations

import pytest

from tests.utils.flows import create_game, join_player

pytestmark = pytest.mark.unit


def test_public_facilitator_id_does_not_authorize_action(api_client):
    code, facilitator_id = create_game(api_client, team_names=["One"])
    join_player(api_client, code, "alice", "team-1")
    status, _, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
        headers={"Cookie": ""}, allow_error=True,
    )
    assert status == 401
    _, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert not state["projects"][0]["started"]


def test_report_requires_facilitator_session(api_client):
    code, _ = create_game(api_client)
    status, _, _ = api_client.json("GET", f"/api/game/{code}/report", allow_error=True)
    assert status == 401


def test_cookie_restores_own_identity_and_cannot_be_replaced_by_public_id(api_client):
    from http.cookies import SimpleCookie

    status, created, headers = api_client.json("POST", "/api/create", {"team_names": ["One"]})
    assert status == 201
    code, facilitator_id = created["game_code"], created["facilitator_id"]
    cookie = SimpleCookie()
    cookie.load(headers["Set-Cookie"])
    assert len(cookie) == 1
    session = next(iter(cookie.values()))
    assert session["httponly"]
    assert session["samesite"] == "Lax"
    assert int(session["max-age"]) == 7 * 24 * 60 * 60
    assert session.value != facilitator_id
    _, facilitator, _ = api_client.json("GET", f"/api/game/{code}/me", actor_id=facilitator_id)
    assert facilitator["id"] == facilitator_id
    assert facilitator["role"] == "facilitator"

    _, joined = join_player(api_client, code, "alice", "team-1")
    player_id = joined["player_id"]
    _, player, _ = api_client.json("GET", f"/api/game/{code}/me", actor_id=player_id)
    assert player["id"] == player_id
    assert player["nickname"] == "alice"
    assert player["role"] == "player"
    for path in ("me", "report", "report?format=csv"):
        status, _, _ = api_client.request("GET", f"/api/game/{code}/{path}", headers={"Cookie": ""}, allow_error=True)
        assert status == 401
    status, _, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"},
        headers=api_client.actor_headers(code, player_id), allow_error=True,
    )
    assert status == 403
    _, state, _ = api_client.json("GET", f"/api/game/{code}")
    assert not state["projects"][0]["started"]

    # The facilitator nickname is reserved too; duplicates never establish a session.
    status, rejected, headers = api_client.json(
        "POST", "/api/join",
        {"game_code": code, "nickname": facilitator["nickname"], "team_id": "team-1"}, allow_error=True,
    )
    assert status == 409
    assert "player_id" not in rejected
    assert "Set-Cookie" not in headers


def test_session_from_another_room_cannot_authorize_read_or_write(api_client):
    code, facilitator_id = create_game(api_client)
    other_code, other_facilitator = create_game(api_client)
    headers = api_client.actor_headers(other_code, other_facilitator)
    for path in ("me", "report", "report?format=csv"):
        status, _, _ = api_client.request("GET", f"/api/game/{code}/{path}", headers=headers, allow_error=True)
        assert status == 401
    status, _, _ = api_client.json(
        "POST", f"/api/game/{code}/start_project",
        {"player_id": facilitator_id, "project_id": "PR-01"}, headers=headers, allow_error=True,
    )
    assert status == 401


def test_report_json_and_csv_require_facilitator_and_describe_real_days(api_client):
    import csv
    import io
    from tests.utils.flows import advance_to_retro

    code, facilitator_id = create_game(api_client, team_names=["One"])
    _, joined = join_player(api_client, code, "alice", "team-1")
    for suffix in ("", "?format=csv"):
        status, _, _ = api_client.request("GET", f"/api/game/{code}/report{suffix}", actor_id=joined["player_id"], allow_error=True)
        assert status == 403
    api_client.json("POST", f"/api/game/{code}/start_project", {"player_id": facilitator_id, "project_id": "PR-01"})
    api_client.json("POST", f"/api/game/{code}/start", {"player_id": facilitator_id})
    retro = advance_to_retro(api_client, code, facilitator_id)
    _, report, _ = api_client.json("GET", f"/api/game/{code}/report", actor_id=facilitator_id)
    assert report["code"] == code
    assert report["revision"] == retro["revision"]
    assert [day["day"] for day in report["days"]] == [1, 2, 3, 4]
    assert report["iterations"][0]["partial"] is False
    assert report["iterations"][0]["throughput"] == 0
    status, body, headers = api_client.request("GET", f"/api/game/{code}/report?format=csv", actor_id=facilitator_id)
    assert status == 200
    assert headers["Content-Type"].startswith("text/csv")
    assert "attachment;" in headers["Content-Disposition"]
    rows = list(csv.DictReader(io.StringIO(body)))
    assert [int(row["day"]) for row in rows] == [1, 2, 3, 4]
    assert all(row["code"] == code for row in rows)
    assert sum(int(row["throughput"]) for row in rows) == 0
    api_client.json("POST", f"/api/game/{code}/continue", {"player_id": facilitator_id})
    _, report, _ = api_client.json("GET", f"/api/game/{code}/report", actor_id=facilitator_id)
    assert report["days"][-1]["day"] == 5
    assert report["days"][-1]["partial"] is True
    assert report["iterations"][-1]["partial"] is True
