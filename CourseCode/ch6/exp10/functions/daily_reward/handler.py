# index.py
import json
import hashlib
from datetime import datetime


def handler(event, context):
    event = json.loads(event)

    player_id = event.get("player_id", "unknown")
    action = event.get("action", "signin")
    timestamp = event.get(
        "timestamp",
        datetime.now().isoformat()
    )

    seed = hashlib.md5(
        f"{player_id}_{timestamp[:10]}".encode()
    ).hexdigest()

    gold = int(seed[:4], 16) % 100 + 10
    exp = int(seed[4:8], 16) % 500 + 50

    streak_day = int(seed[8:10], 16) % 7 + 1
    streak_bonus = (
        streak_day * 10
        if streak_day > 3
        else 0
    )

    result = {
        "player_id": player_id,
        "action": action,
        "timestamp": timestamp,
        "reward": {
            "gold": gold + streak_bonus,
            "exp": exp,
            "streak_day": streak_day,
            "streak_bonus": streak_bonus,
            "item": (
                "lucky_box"
                if int(seed[10:12],16) > 200
                else None
            )
        },
        "message":
            f"签到成功！获得 {gold + streak_bonus} 金币，"
            f"{exp} 经验，连续签到第 {streak_day} 天"
    }

    return json.dumps(result, ensure_ascii=False)