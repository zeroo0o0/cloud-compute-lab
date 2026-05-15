#!/usr/bin/env python3
"""
每日签到奖励函数 - Serverless Handler
模拟 FaaS 函数：接收签到事件，返回奖励结果
"""
import json
import sys
import hashlib
from datetime import datetime

def handler(event, context=None):
    """签到奖励入口函数"""
    player_id = event.get("player_id", "unknown")
    action = event.get("action", "signin")
    timestamp = event.get("timestamp", datetime.now().isoformat())

    # 基于 player_id 生成确定性但看起来随机的奖励
    seed = hashlib.md5(f"{player_id}_{timestamp[:10]}".encode()).hexdigest()
    gold = int(seed[:4], 16) % 100 + 10  # 10-109 金币
    exp = int(seed[4:8], 16) % 500 + 50   # 50-549 经验

    # 连续签到奖励
    streak_day = int(seed[8:10], 16) % 7 + 1
    streak_bonus = streak_day * 10 if streak_day > 3 else 0

    return {
        "player_id": player_id,
        "action": action,
        "timestamp": timestamp,
        "reward": {
            "gold": gold + streak_bonus,
            "exp": exp,
            "streak_day": streak_day,
            "streak_bonus": streak_bonus,
            "item": "lucky_box" if int(seed[10:12], 16) > 200 else None,
        },
        "message": f"签到成功！获得 {gold + streak_bonus} 金币，{exp} 经验，连续签到第 {streak_day} 天"
    }

if __name__ == "__main__":
    if len(sys.argv) > 1:
        event = json.loads(sys.argv[1])
    else:
        event = {"player_id": "test_player", "action": "signin", "timestamp": datetime.now().isoformat()}

    result = handler(event)
    print(json.dumps(result, ensure_ascii=False))
